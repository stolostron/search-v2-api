// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"sync"
	"time"

	"github.com/stolostron/search-v2-api/pkg/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

type userData struct {
	// impersonate kubernetes.Interface //client with impersonation config
	err        error
	namespaces []string
	// resources  map[string][]string //key:namespace value list of resources
	updatedAt time.Time
	lock      sync.Mutex
}

func (cache *Cache) GetUserData(ctx context.Context, clientToken string) (*userData, error) {
	var user userData
	uid := cache.tokenReviews[clientToken].tokenReview.Status.User.UID //get uid from tokenreview
	cache.userLock.Lock()
	defer cache.userLock.Unlock()
	cachedUserData, userDataExists := cache.users[uid] //check if userData cache for user already exists
	if userDataExists {
		klog.V(5).Info("Using user data from cache.")
		return cachedUserData, cachedUserData.err

	}
	// create new instance
	cache.users[uid] = &user
	userData, err := user.getNamespaces(cache, ctx, clientToken)
	return userData, err

}

func (user *userData) getNamespaces(cache *Cache, ctx context.Context, clientToken string) (*userData, error) {
	//lock to prevent checking more than one at a time
	user.lock.Lock()
	defer user.lock.Unlock()
	if len(user.namespaces) > 0 &&
		time.Now().Before(user.updatedAt.Add(time.Duration(config.Cfg.UserCacheTTL)*time.Millisecond)) {
		klog.V(5).Info("Using user's namespaces from cache.")
		return user, user.err
	}

	klog.V(5).Info("Getting namespaces from shared cache.")
	allNamespaces := cache.shared.namespaces
	user.err = nil

	var impersonNamespaces []string
	for _, ns := range allNamespaces {

		impersonationClientset := cache.getImpersonationClientSet(clientToken, cache.resConfig)
		v1Namespaces, kubeErr := impersonationClientset.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
		if kubeErr != nil {
			klog.Warning("Error resolving namespaces from KubeClient: ", kubeErr)
		}

		impersonNamespaces = append(impersonNamespaces, v1Namespaces.Name)

	}
	user.namespaces = append(user.namespaces, impersonNamespaces...)
	klog.Info("We can impersonate user for these namespaces:", impersonNamespaces)

	user.updatedAt = time.Now()
	return user, user.err
}

func (cache *Cache) getImpersonationClientSet(clientToken string, config *rest.Config) kubernetes.Interface {

	config.Impersonate = rest.ImpersonationConfig{
		UID: cache.tokenReviews[clientToken].tokenReview.Status.User.UID,
	}

	clientset, err := kubernetes.NewForConfig(cache.resConfig)
	if err != nil {
		klog.Info("Error with creating a new clientset with impersonation config.", err.Error())
	}

	cache.kubeClient = clientset

	return cache.kubeClient
}
