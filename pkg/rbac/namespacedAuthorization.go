// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"sync"
	"time"

	"github.com/stolostron/search-v2-api/pkg/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

type userData struct {
	err        error
	namespaces []string
	updatedAt  time.Time
	lock       sync.Mutex
}

func (cache *Cache) NamespacedResources(ctx context.Context, clientToken string) (*userData, error) {
	uid := cache.tokenReviews[clientToken].tokenReview.Status.User.UID //get uid from tokenreview

	cachedUserData, userDataExists := cache.users[uid] //check if userdata cache exists
	if userDataExists {
		klog.V(5).Info("Using user data from cache.")
		return cachedUserData, cachedUserData.err

	}

	// create new instance and
	user := cache.users[uid]
	user = &userData{}
	userData, err := user.getNamespacedResources(cache, ctx)
	return userData, err

}

func (user *userData) getNamespacedResources(cache *Cache, ctx context.Context) (*userData, error) {
	//lock to prevent checking more than one at a time

	user.lock.Lock()
	defer user.lock.Unlock()
	if len(user.namespaces) > 0 && time.Now().Before(user.updatedAt.Add(time.Duration(config.Cfg.UserCacheTTL)*time.Millisecond)) {
		klog.V(5).Info("Using user's namespaces from cache.")
		return user, user.err
	}

	klog.V(5).Info("Getting namespaces from Kube Client")
	user.err = nil

	var userNamespaces []string
	namespaceList, kubeErr := config.KubeClient().CoreV1().Namespaces().List(ctx, metav1.ListOptions{}) // get all namespaces in cluster
	if kubeErr != nil {
		klog.Warning("Error resolving namespaces from KubeClient: ", kubeErr)
		user.err = kubeErr
		user.namespaces = []string{}
		return user, kubeErr
	}

	for _, n := range namespaceList.Items {
		userNamespaces = append(userNamespaces, n.Name)
	}

	user.namespaces = userNamespaces
	user.updatedAt = time.Now()
	return user, user.err
}
