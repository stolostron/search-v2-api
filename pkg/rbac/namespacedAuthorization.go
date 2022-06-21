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

func (cache *Cache) NamespacedResources(ctx context.Context, clientToken string) ([]string, error) {
	uid := cache.tokenReviews[clientToken].tokenReview.Status.User.UID
	namespaces, err := cache.users[uid].getNamespacedResources(cache, ctx)
	return namespaces, err

}

func (user *userData) getNamespacedResources(cache *Cache, ctx context.Context) ([]string, error) {
	user.lock.Lock()
	defer user.lock.Unlock()
	if user.namespaces != nil &&
		time.Now().Before(user.updatedAt.Add(time.Duration(config.Cfg.UserCacheTTL)*time.Millisecond)) {
		klog.V(5).Info("Using user's namespaces from cache.")
		return user.namespaces, user.err
	}

	klog.V(5).Info("Getting namespaces from Kube Client")
	user.err = nil

	var userNamespaces []string
	namespaceList, kubeErr := config.KubeClient().CoreV1().Namespaces().List(ctx, metav1.ListOptions{}) // get all namespaces in cluster
	if kubeErr != nil {
		klog.Warning("Error resolving namespaces from KubeClient: ", kubeErr)
		user.err = kubeErr
		user.namespaces = []string{}
		return user.namespaces, kubeErr
	}

	for _, n := range namespaceList.Items {
		userNamespaces = append(userNamespaces, n.Name)
	}

	user.namespaces = userNamespaces
	user.updatedAt = time.Now()
	return user.namespaces, user.err
}
