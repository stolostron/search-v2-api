package rbac

import (
	"testing"
)

// Initialize cache object to use tests.
// func newCache() Cache {
// 	return Cache{
// 		// Use a fake Kubernetes authentication client.
// 		authClient:          fake.NewSimpleClientset().AuthenticationV1(),
// 		tokenReviews:        map[string]*tokenReviewResult{},
// 		tokenReviewsPending: map[string][]chan *tokenReviewResult{},
// 		tokenReviewsLock:    sync.Mutex{},
// 	}
// }

func Test_GetResourcesIfEmptyCache(t *testing.T) {

	// // Initialize cache with empty state.
	// cache := newCache()

	// // Execute function
	// result, err := cache.IsValidToken(context.TODO(), "1234567890")

	// // Validate results
	// if result {
	// 	t.Error("Expected token to be invalid.")
	// }
	// if err != nil {
	// 	t.Error("Received unexpected error from IsValidToken()", err)
	// }

}
