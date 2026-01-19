// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"errors"
	"fmt"
	"github.com/stolostron/search-v2-api/pkg/config"
	authv1 "k8s.io/api/authentication/v1"
	authz "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sync"
	"time"

	v1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
)

type WatchCache struct {
	WatchUserDataLock   sync.Mutex
	WatchUserData       map[string]*UserWatchData // userID: UserWatchPermissions
	WatchCacheUpdatedAt time.Time
}

type UserWatchData struct {
	AuthzClient     v1.AuthorizationV1Interface
	Permissions     map[WatchPermissionKey]*WatchPermissionEntry
	PermissionsLock sync.RWMutex
	ttl             time.Duration
}

type WatchPermissionKey struct {
	Verb      string
	Apigroup  string
	Kind      string
	Namespace string
}

type WatchPermissionEntry struct {
	Allowed   bool
	UpdatedAt time.Time
}

var watchCacheInst = WatchCache{}

func (w *WatchCache) isValid() bool {
	return true
}

func (w *WatchCache) GetUserWatchData(ctx context.Context) (*UserWatchData, error) {
	return w.GetUserWatchDataCache(ctx, nil)
}

func (w *WatchCache) GetUserWatchDataCache(ctx context.Context, authzClient v1.AuthorizationV1Interface) (*UserWatchData, error) {
	uid, userInfo, err := getUserUidFromContext(ctx)
	if err != nil {
		return nil, err
	}

	w.WatchUserDataLock.Lock()
	defer w.WatchUserDataLock.Unlock()

	if w.WatchUserData == nil {
		w.WatchUserData = make(map[string]*UserWatchData)
	}

	// check if user exists in cache
	if userData, ok := w.WatchUserData[uid]; ok {
		return userData, nil
	}

	// init user entry in cache with permissions structs and authzClient for making SSAR calls
	userData := &UserWatchData{
		Permissions:     make(map[WatchPermissionKey]*WatchPermissionEntry),
		PermissionsLock: sync.RWMutex{},
		ttl:             time.Duration(config.Cfg.UserCacheTTL) * time.Millisecond,
	}

	if authzClient != nil {
		userData.AuthzClient = authzClient
	} else {
		userData.AuthzClient = createImpersonationClient(userInfo)
		if userData.AuthzClient == nil {
			return nil, fmt.Errorf("failed to create impersonation client for user %s", uid)
		}
	}

	w.WatchUserData[uid] = userData

	return userData, nil
}

func GetWatchCache() *WatchCache {
	return &watchCacheInst
}

func (u *UserWatchData) userAuthorizedWatchSSAR(ctx context.Context, authzClient v1.AuthorizationV1Interface, verb, apigroup, kind, namespace string) bool {
	accessCheck := &authz.SelfSubjectAccessReview{
		Spec: authz.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authz.ResourceAttributes{
				Verb:      verb,
				Group:     apigroup,
				Resource:  kind,
				Namespace: namespace,
			},
		},
	}
	result, err := authzClient.SelfSubjectAccessReviews().Create(ctx, accessCheck, metav1.CreateOptions{})
	if err != nil {
		klog.Errorf("Error during watch self subject access review: %v", err)
		return false
	}
	klog.V(6).Infof("SelfSubjectAccessReview watch result for verb=%s resource=%s group=%s namespace=%s: %v",
		verb, kind, apigroup, namespace, result.Status.Allowed)
	return result.Status.Allowed
}

func (u *UserWatchData) CheckPermissionAndCache(ctx context.Context, verb, apigroup, kind, namespace string) bool {
	key := WatchPermissionKey{
		Verb:      verb,
		Apigroup:  apigroup,
		Kind:      kind,
		Namespace: namespace,
	}

	needsInvalidation := false
	// check cache for record and return if cache ttl still valid
	u.PermissionsLock.RLock()
	if entry, ok := u.Permissions[key]; ok {
		if time.Since(entry.UpdatedAt) < u.ttl {
			klog.V(6).Infof("Using cached watch permission: %+v = %v", key, entry.Allowed)
			u.PermissionsLock.RUnlock()
			return entry.Allowed
		} else {
			u.PermissionsLock.RUnlock()
			needsInvalidation = true
		}
	}

	klog.V(6).Infof("Cache miss for watch permission: %+v. Making SSAR call.", key)
	allowed := u.userAuthorizedWatchSSAR(ctx, u.AuthzClient, verb, apigroup, kind, namespace)

	// store in result cache
	u.PermissionsLock.Lock()
	defer u.PermissionsLock.Unlock()
	if u.Permissions == nil || needsInvalidation {
		u.Permissions = make(map[WatchPermissionKey]*WatchPermissionEntry)
	}
	u.Permissions[key] = &WatchPermissionEntry{
		Allowed:   allowed,
		UpdatedAt: time.Now(),
	}
	klog.V(6).Infof("Cached watch permission: %+v = %v", key, allowed)

	return allowed
}

func getUserUidFromContext(ctx context.Context) (string, authv1.UserInfo, error) {
	// we need a regular cache instance to be able to reuse token review code to build impersonation client
	cache := GetCache()
	uid, userInfo := cache.GetUserUID(ctx)
	if uid == rbacNoUidFound {
		return "", authv1.UserInfo{}, errors.New("failed to get uid from context")
	}

	return uid, userInfo, nil
}

func createImpersonationClient(userInfo authv1.UserInfo) v1.AuthorizationV1Interface {
	klog.V(5).Infof("Creating impersonation client for user %s", userInfo.Username)

	restConfig := config.GetClientConfig()

	impersonationConfig := &rest.ImpersonationConfig{}
	if userInfo.Username != "" {
		impersonationConfig.UserName = userInfo.Username
	}
	if userInfo.UID != "" {
		impersonationConfig.UID = userInfo.UID
	}
	if len(userInfo.Groups) > 0 {
		impersonationConfig.Groups = userInfo.Groups
	}
	if len(userInfo.Extra) > 0 {
		extraUpdated := map[string][]string{}
		for key, val := range userInfo.Extra {
			extraUpdated[key] = val
		}
		impersonationConfig.Extra = extraUpdated
	}

	restConfig.Impersonate = *impersonationConfig

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		klog.Errorf("Error creating clientset with impersonation config: %v", err)
		return nil
	}

	return clientset.AuthorizationV1()
}
