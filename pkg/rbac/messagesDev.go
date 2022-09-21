// Copyright Contributors to the Open Cluster Management project
//go:build test
// +build test

// This file is excluded from compilation unless the build flag -tags test is used.
// Use `make test` to run with the test flag.
// This file is added to share mock test functions with resolver package as per solution in
// https://stackoverflow.com/questions/44192688/how-to-share-test-interfaces-between-go-packages

package rbac

import (
	"sync"
	"time"

	"github.com/driftprogramming/pgxpoolmock"
	authv1 "k8s.io/api/authentication/v1"
	fake "k8s.io/client-go/kubernetes/fake"
)

// Initialize cache object to use tests.
func NewMockCacheForMessages(dc, mc map[string]struct{}, mockPool *pgxpoolmock.MockPgxPool) *Cache {
	user := map[string]*UserDataCache{}
	user["unique-user-id"] = &UserDataCache{
		userData:          UserData{ManagedClusters: mc},
		clustersUpdatedAt: time.Now(),
		csrUpdatedAt:      time.Now(),
		nsrUpdatedAt:      time.Now()}

	cache := Cache{
		pool: mockPool,
		// Use a fake Kubernetes authentication client.
		authnClient:      fake.NewSimpleClientset().AuthenticationV1(),
		tokenReviews:     map[string]*tokenReviewCache{},
		tokenReviewsLock: sync.Mutex{},
		shared: SharedData{
			disabledClusters: dc,
			managedClusters:  mc,
		},
		users: user,
	}
	if dc != nil {
		cache.shared.dcUpdatedAt = time.Now()
	}
	if mc != nil {
		cache.shared.mcUpdatedAt = time.Now()
	}
	return SetupTokenForMessages(&cache)
}

func SetupTokenForMessages(cache *Cache) *Cache {
	if cache.tokenReviews == nil {
		cache.tokenReviews = map[string]*tokenReviewCache{}
	}
	cache.tokenReviews["123456"] = &tokenReviewCache{
		updatedAt:  time.Now(),
		authClient: fake.NewSimpleClientset().AuthenticationV1(),
		tokenReview: &authv1.TokenReview{
			Status: authv1.TokenReviewStatus{
				User: authv1.UserInfo{
					UID: "unique-user-id",
				},
			},
		},
	}
	return cache
}
