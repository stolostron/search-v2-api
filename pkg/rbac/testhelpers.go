// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"time"

	authv1 "k8s.io/api/authentication/v1"
)

// SetupMockClusterAdmin sets up the cache with a mock cluster admin user for testing
func SetupMockClusterAdmin(cache *Cache, token string, uid string) {
	if cache.tokenReviews == nil {
		cache.tokenReviews = map[string]*tokenReviewCache{}
	}

	// setup token review
	cache.tokenReviews[token] = &tokenReviewCache{
		meta: cacheMetadata{updatedAt: time.Now()},
		tokenReview: &authv1.TokenReview{
			Status: authv1.TokenReviewStatus{
				User: authv1.UserInfo{
					UID:      uid,
					Username: "cluster-admin",
				},
			},
		},
	}

	// setup cache and cluster admin
	if cache.users == nil {
		cache.users = map[string]*UserDataCache{}
	}
	cache.users[uid] = &UserDataCache{
		UserData: UserData{
			IsClusterAdmin: true,
		},
		csrCache:      cacheMetadata{updatedAt: time.Now()},
		fgRbacNsCache: cacheMetadata{updatedAt: time.Now()},
		nsrCache:      cacheMetadata{updatedAt: time.Now()},
		clustersCache: cacheMetadata{updatedAt: time.Now()},
	}
}
