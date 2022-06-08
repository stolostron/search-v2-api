// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/stolostron/search-v2-api/pkg/config"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/kubernetes/typed/authentication/v1"
	"k8s.io/klog/v2"
)

// Encapsulates a TokenReview to store in the cache.
type tokenReviewCache struct {
	authClient  v1.AuthenticationV1Interface // This allows tests to replace with mock client.
	err         error
	lock        sync.Mutex
	updatedAt   time.Time
	token       string
	tokenReview *authv1.TokenReview
}

// Verify that the token is valid using a TokenReview.
// Will use cached data if available and valid, otherwise starts a new request.
func (c *Cache) IsValidToken(ctx context.Context, token string) (bool, error) {
	tr, err := c.getTokenReview(ctx, token)
	return tr.Status.Authenticated, err
}

// Get the TokenReview response for a given token.
// Will use cached data if available and valid, otherwise starts a new request.
func (c *Cache) getTokenReview(ctx context.Context, token string) (*authv1.TokenReview, error) {
	c.tokenReviewsLock.Lock()
	defer c.tokenReviewsLock.Unlock()

	// Check if a TokenReviewCacheRequest exists in the cache or create a new one.
	cachedTR, tokenExists := c.tokenReviews[token]
	if !tokenExists {
		cachedTR = &tokenReviewCache{
			authClient: c.getAuthClient(),
			token:      token,
		}
		c.tokenReviews[token] = cachedTR
	}
	return cachedTR.getTokenReview()
}

// Get the resolved TokenReview from the cached tokenReviewCachedRequest object.
func (trc *tokenReviewCache) getTokenReview() (*authv1.TokenReview, error) {
	// This ensures that only 1 process is updating the TokenReview data from API request.
	trc.lock.Lock()
	defer trc.lock.Unlock()

	// Check if cached TokenReview data is valid. Update if needed.
	if time.Now().After(trc.updatedAt.Add(time.Duration(config.Cfg.AuthCacheTTL) * time.Millisecond)) {
		klog.V(5).Infof("Starting TokenReview. tokenReviewCache expired or never updated. UpdatedAt %s", trc.updatedAt)

		tr := authv1.TokenReview{
			Spec: authv1.TokenReviewSpec{
				Token: trc.token,
			},
		}

		result, err := trc.authClient.TokenReviews().Create(context.TODO(), &tr, metav1.CreateOptions{})
		if err != nil {
			klog.Warning("Error resolving TokenReview from Kube API.", err.Error())
		}
		klog.V(9).Infof("TokenReview Kube API result: %v\n", prettyPrint(result.Status))

		trc.updatedAt = time.Now()
		trc.err = err
		trc.tokenReview = result
	} else {
		klog.V(6).Info("Using cached TokenReview.")
	}

	return trc.tokenReview, trc.err
}

// https://stackoverflow.com/a/51270134
func prettyPrint(i interface{}) string {
	s, _ := json.MarshalIndent(i, "", "\t")
	return string(s)
}

// Utility to allow tests to inject a fake client to mock the k8s api call.
func (c *Cache) getAuthClient() v1.AuthenticationV1Interface {
	if c.authClient == nil {
		c.authClient = config.KubeClient().AuthenticationV1()
	}
	return c.authClient
}
