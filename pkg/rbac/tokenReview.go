// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"encoding/json"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stolostron/search-v2-api/pkg/config"
	"github.com/stolostron/search-v2-api/pkg/metric"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/kubernetes/typed/authentication/v1"
	"k8s.io/klog/v2"
)

// Encapsulates a TokenReview to store in the cache.
type tokenReviewCache struct {
	meta cacheMetadata

	authClient  v1.AuthenticationV1Interface // This allows tests to replace with mock client.
	token       string
	tokenReview *authv1.TokenReview
}

// Verify that the token is valid using a TokenReview.
// Will use cached data if available and valid, otherwise starts a new request.
func (c *Cache) IsValidToken(ctx context.Context, token string) (bool, error) {
	tr, err := c.GetTokenReview(ctx, token)
	return tr.Status.Authenticated, err
}

// Get the TokenReview response for a given token.
// Will use cached data if available and valid, otherwise starts a new request.
func (c *Cache) GetTokenReview(ctx context.Context, token string) (*authv1.TokenReview, error) {
	c.tokenReviewsLock.Lock()
	defer c.tokenReviewsLock.Unlock()

	// Check if a TokenReviewCacheRequest exists in the cache or create a new one.
	cachedTR, tokenExists := c.tokenReviews[token]
	if !tokenExists {
		//create observation for cache being created with label authentication
		//create metric and set labels
		HttpDurationByQuery := metric.HttpDurationByLabels(prometheus.Labels{"action": "create_token_review"})

		//create timer and return observed duration
		timer := prometheus.NewTimer(HttpDurationByQuery.WithLabelValues("200")) //change labels
		defer timer.ObserveDuration()

		cachedTR = &tokenReviewCache{
			authClient: c.getAuthClient(),
			token:      token,
		}
		if c.tokenReviews == nil {
			c.tokenReviews = map[string]*tokenReviewCache{}
		}
		c.tokenReviews[token] = cachedTR
	}
	return cachedTR.getTokenReview()
}

// Get the resolved TokenReview from the cached tokenReviewCachedRequest object.
func (trc *tokenReviewCache) getTokenReview() (*authv1.TokenReview, error) {
	// This ensures that only 1 process is updating the TokenReview data from API request.
	trc.meta.lock.Lock()
	defer trc.meta.lock.Unlock()
	// var timer *prometheus.Timer

	// Check if cached TokenReview data is valid. Update if needed.
	if time.Now().After(trc.meta.updatedAt.Add(time.Duration(config.Cfg.AuthCacheTTL) * time.Millisecond)) {
		// defer timer.ObserveDuration() // record time passed since timer created - in our case recording time until new token session (or total time of user session before renew)
		klog.V(6).Infof("Starting TokenReview. tokenReviewCache expired or never updated. UpdatedAt %s", trc.meta.updatedAt)

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

		trc.meta.updatedAt = time.Now()
		trc.meta.err = err
		trc.tokenReview = result
		// timer = prometheus.NewTimer(metric.HttpDuration.WithLabelValues("totalUserSessionDuration"))

	} else {
		klog.V(6).Info("Using cached TokenReview.")
		// timer = prometheus.NewTimer(metric.HttpDuration.WithLabelValues("totalUserSessionDuration"))

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
