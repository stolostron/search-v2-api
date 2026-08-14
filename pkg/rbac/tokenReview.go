// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/stolostron/search-v2-api/pkg/config"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/kubernetes/typed/authentication/v1"
	"k8s.io/klog/v2"
)

// hashTokenKey is a process-local secret used to key the hashToken HMAC.
// It is generated once at process startup and never persisted or exposed,
// so cache keys derived from it cannot be precomputed or reversed offline.
// It only needs to be stable for the lifetime of the process, since the
// tokenReviews cache itself is in-memory only.
var hashTokenKey = newHashTokenKey()

func newHashTokenKey() []byte {
	return newHashTokenKeyFrom(rand.Reader)
}

// newHashTokenKeyFrom generates the HMAC key by reading from randReader.
// Split out from newHashTokenKey so the CSPRNG-failure fallback path can be
// exercised in tests without depending on crypto/rand actually failing.
func newHashTokenKeyFrom(randReader io.Reader) []byte {
	key := make([]byte, 32)
	if _, err := io.ReadFull(randReader, key); err != nil {
		// Extremely unlikely: crypto/rand failed to read from the OS CSPRNG.
		// Fall back to a fixed key so the process can still start; this only
		// weakens the secrecy of the HMAC key, not the correctness of the cache.
		klog.Warning("Failed to generate random key for hashToken, using fallback key.", err)
		for i := range key {
			key[i] = byte(i)
		}
	}
	return key
}

// Encapsulates a TokenReview to store in the cache.
type tokenReviewCache struct {
	meta cacheMetadata

	authClient  v1.AuthenticationV1Interface // This allows tests to replace with mock client.
	tokenReview *authv1.TokenReview
}

// Verify that the token is valid using a TokenReview.
// Will use cached data if available and valid, otherwise starts a new request.
func (c *Cache) IsValidToken(ctx context.Context, token string) (bool, error) {
	tr, err := c.GetTokenReview(ctx, token)
	return tr.Status.Authenticated, err
}

// hashToken returns a keyed HMAC-SHA256 hex digest of a raw token value.
// Used as the cache key to avoid retaining bearer tokens in heap memory.
// A per-process HMAC key (rather than a bare hash) is used because the
// raw token is sensitive data; keying the hash prevents an attacker who
// only observes the cache keys from precomputing or reversing them.
func hashToken(token string) string {
	mac := hmac.New(sha256.New, hashTokenKey)
	mac.Write([]byte(token))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

// Get the TokenReview response for a given token.
// Will use cached data if available and valid, otherwise starts a new request.
func (c *Cache) GetTokenReview(ctx context.Context, token string) (*authv1.TokenReview, error) {
	c.tokenReviewsLock.Lock()
	defer c.tokenReviewsLock.Unlock()

	if c.tokenReviews == nil {
		c.tokenReviews = map[string]*tokenReviewCache{}
	}

	// Evict all entries whose TTL has expired before doing anything else, so
	// the map never retains stale data beyond the configured window.
	c.evictExpiredTokenReviews()

	// Check if a TokenReviewCacheRequest exists in the cache or create a new one.
	tokenHash := hashToken(token)
	cachedTR, tokenExists := c.tokenReviews[tokenHash]
	if !tokenExists {
		// Enforce the size cap before inserting: when we are at the limit,
		// evict the single oldest entry to make room.
		if len(c.tokenReviews) >= config.Cfg.AuthCacheMaxSize {
			c.evictOldestTokenReview()
		}
		cachedTR = &tokenReviewCache{
			authClient: c.getAuthClient(),
		}
		c.tokenReviews[tokenHash] = cachedTR
	}
	return cachedTR.getTokenReview(token)
}

// evictExpiredTokenReviews removes all cache entries whose updatedAt timestamp
// is older than the configured AuthCacheTTL. Must be called with tokenReviewsLock held.
// The per-entry meta.lock is not acquired here because tokenReviewsLock already
// serialises all access to the map and its entries at this call site.
func (c *Cache) evictExpiredTokenReviews() {
	ttl := time.Duration(config.Cfg.AuthCacheTTL) * time.Millisecond
	cutoff := time.Now().Add(-ttl)
	for key, entry := range c.tokenReviews {
		if !entry.meta.updatedAt.IsZero() && entry.meta.updatedAt.Before(cutoff) {
			delete(c.tokenReviews, key)
			klog.V(7).Infof("Evicted expired TokenReview entry from cache.")
		}
	}
}

// evictOldestTokenReview removes the single cache entry with the earliest
// updatedAt time. Must be called with tokenReviewsLock held.
// The per-entry meta.lock is not acquired here because tokenReviewsLock already
// serialises all access to the map and its entries at this call site.
func (c *Cache) evictOldestTokenReview() {
	var oldestKey string
	var oldestTime time.Time
	for key, entry := range c.tokenReviews {
		if oldestKey == "" || entry.meta.updatedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.meta.updatedAt
		}
	}
	if oldestKey != "" {
		delete(c.tokenReviews, oldestKey)
		klog.V(7).Infof("Evicted oldest TokenReview entry to enforce cache size limit of %d.", config.Cfg.AuthCacheMaxSize)
	}
}

// Get the resolved TokenReview from the cached tokenReviewCachedRequest object.
// The raw token is passed transiently and never stored on the struct.
func (trc *tokenReviewCache) getTokenReview(token string) (*authv1.TokenReview, error) {
	// This ensures that only 1 process is updating the TokenReview data from API request.
	trc.meta.lock.Lock()
	defer trc.meta.lock.Unlock()

	// Check if cached TokenReview data is valid. Update if needed.
	if time.Now().After(trc.meta.updatedAt.Add(time.Duration(config.Cfg.AuthCacheTTL) * time.Millisecond)) {
		klog.V(6).Infof("Starting TokenReview. tokenReviewCache expired or never updated. UpdatedAt %s", trc.meta.updatedAt)

		tr := authv1.TokenReview{
			Spec: authv1.TokenReviewSpec{
				Token: token,
			},
		}

		result, err := trc.authClient.TokenReviews().Create(context.TODO(), &tr, metav1.CreateOptions{})
		if err != nil {
			klog.Warning("Error resolving TokenReview from Kube API.", err.Error())
		}
		klog.V(9).Infof("TokenReview Kube API result: %v\n", prettyPrint(result.Status))

		trc.meta.updatedAt = time.Now()
		trc.meta.err = err
		trc.tokenReview = result
	} else {
		klog.V(6).Info("Using cached TokenReview.")
	}

	return trc.tokenReview, trc.meta.err
}

// https://stackoverflow.com/a/51270134
func prettyPrint(i interface{}) string {
	s, _ := json.MarshalIndent(i, "", "\t")
	return string(s)
}

// Utility to allow tests to inject a fake client to mock the k8s api call.
func (c *Cache) getAuthClient() v1.AuthenticationV1Interface {
	if c.authnClient == nil {
		c.authnClient = config.KubeClient().AuthenticationV1()
	}
	return c.authnClient
}
