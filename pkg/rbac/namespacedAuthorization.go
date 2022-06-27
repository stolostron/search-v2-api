// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/stolostron/search-v2-api/pkg/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

type userData struct {
	impersonate kubernetes.Interface //client with impersonation config
	err         error
	namespaces  []string
	updatedAt   time.Time
	lock        sync.Mutex
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
	userData, err := user.getNamespacedResources(cache, ctx, clientToken)
	return userData, err

}

func (user *userData) getNamespacedResources(cache *Cache, ctx context.Context, clientToken string) (*userData, error) {
	//lock to prevent checking more than one at a time
	user.lock.Lock()
	defer user.lock.Unlock()
	if len(user.namespaces) > 0 &&
		time.Now().Before(user.updatedAt.Add(time.Duration(config.Cfg.UserCacheTTL)*time.Millisecond)) {
		klog.V(5).Info("Using user's namespaces from cache.")
		return user, user.err
	}

	klog.V(5).Info("Getting namespaces from Kube Client")
	user.err = nil

	clientset := cache.getClientSet(clientToken)
	user.impersonate = clientset

	var userNamespaces []string
	namespaceList, kubeErr := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
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

func (cache *Cache) getClientSet(clientToken string) kubernetes.Interface {

	cache.resConfig = config.GetClientConfig()
	cache.resConfig.Impersonate = rest.ImpersonationConfig{
		UserName: cache.tokenReviews[clientToken].tokenReview.Status.User.Username,
	}
	clientset, err := kubernetes.NewForConfig(cache.resConfig)
	if err != nil {
		fmt.Println("Error with creating a new clientset with impersonation config.", err.Error())
	}

	cache.kubeClient = clientset

	return cache.kubeClient
}
