// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"sync"
	"time"

	"github.com/stolostron/search-v2-api/pkg/config"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

var cachedTokenReview = CachedTokenReview{
	pending: map[string][]chan *tokenReviewResult{},
	cache:   map[string]*tokenReviewResult{},
	lock:    sync.Mutex{},
}

type tokenReviewResult struct {
	t           time.Time
	tokenReview *authv1.TokenReview
}

type CachedTokenReview struct {
	pending map[string][]chan *tokenReviewResult
	cache   map[string]*tokenReviewResult
	lock    sync.Mutex
}

func (cache *CachedTokenReview) IsValidToken(token string) bool {
	tr := cache.getTokenReview(token)

	return tr.Status.Authenticated
}

func (cache *CachedTokenReview) getTokenReview(token string) *authv1.TokenReview {
	cache.lock.Lock()
	// defer cache.lock.Unlock()
	result := make(chan *tokenReviewResult)

	// 1. Check if we can use TokenReview from the cache.
	tr, tokenExists := cache.cache[token]
	if tokenExists && time.Now().Before(tr.t.Add(60*time.Second)) {
		klog.V(5).Info("Using TokenReview from cache.")
		cache.lock.Unlock()
		return tr.tokenReview
	}
	// else if !tokenExists {
	// 	klog.Info("TokenReview is not in the cache.")
	// } else if time.Now().After(tr.t.Add(60 * time.Second)) {
	// 	klog.Info("Cached TokenReview is older than 60 seconds.")
	// }

	// 2. Check if there's a pending request
	pending, isPending := cache.pending[token]
	if isPending {
		klog.V(5).Info("Found a pending TokenReview request for this token. Adding channel to get notified when resolved.")
		cache.pending[token] = append(pending, result)
	} else {
		klog.V(5).Info("Triggering a new TokenReview.")
		go cache.asyncTokenReview(context.TODO(), token, result)
	}

	// Wait until the TokenReview request gets resolved.
	cache.lock.Unlock()
	tr = <-result
	return tr.tokenReview
}

func (cache *CachedTokenReview) asyncTokenReview(ctx context.Context, token string, ch chan *tokenReviewResult) {
	cache.lock.Lock()
	pending, foundPending := cache.pending[token]
	if foundPending {
		cache.pending[token] = append(cache.pending[token], ch)
	} else {
		cache.pending[token] = []chan *tokenReviewResult{ch}
	}
	cache.lock.Unlock()

	tr := authv1.TokenReview{
		Spec: authv1.TokenReviewSpec{
			Token: token,
		},
	}
	result, err := config.KubeClient().AuthenticationV1().TokenReviews().Create(ctx, &tr, metav1.CreateOptions{})

	cache.lock.Lock()
	defer cache.lock.Unlock()

	// Refresh the channels to send the response in case a new channel was added.
	pending, _ = cache.pending[token]

	if err != nil {
		klog.Warning("Error during TokenReview. ", err.Error())

		// TODO: May need better error handling logic.
		for _, p := range pending {
			close(p)
		}
	}

	resultMsg := &tokenReviewResult{t: time.Now(), tokenReview: result}
	for _, p := range pending {
		p <- resultMsg
		close(p)
	}

	delete(cache.pending, token)
	cache.cache[token] = resultMsg
}