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
	updatedAt  time.Time
	lock       sync.Mutex
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

	klog.V(5).Info("Checking shared cache for namespaces..")
	user.err = nil

	var allNamespaces []string
	if len(cache.shared.namespaces) > 0 {
		klog.V(5).Info("Using namespaces from shared cache")
		allNamespaces = append(allNamespaces, cache.shared.namespaces...)

	} else {

		klog.V(5).Info("Getting namespaces from Kube Client..")

		//getting rest.Config
		cache.resConfig = config.GetClientConfig()
		//settign kubernetes client
		clientset, err := kubernetes.NewForConfig(cache.resConfig)
		if err != nil {
			klog.Info("Error with creating a new clientset with impersonation config.", err.Error())
		}

		// get all shared namespaces using kubeclient with rest.Config:
		namespaceList, kubeErr := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
		if kubeErr != nil {
			klog.Warning("Error resolving namespaces from KubeClient: ", kubeErr)
			user.err = kubeErr
			user.namespaces = []string{}
			return user, kubeErr //if there is an Kube client error return user struct and error
		}

		// add namespaces to allNamespace List
		for _, n := range namespaceList.Items {
			allNamespaces = append(allNamespaces, n.Name)
		}
		// cache to shared:
		cache.shared.namespaces = allNamespaces
	}

	var impersonNamespaces []string
	for _, ns := range allNamespaces {

		impersonationClientset := cache.getImpersonationClientSet(clientToken, cache.resConfig)
		// .List(ctx, metav1.ListOptions{})
		v1Namespaces, kubeErr := impersonationClientset.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{}) //impersonating user for each namespace
		//if we have error
		if kubeErr != nil {
			klog.Warning("Error resolving namespaces from KubeClient: ", kubeErr)
		}

		impersonNamespaces = append(impersonNamespaces, v1Namespaces.Name)

	}
	klog.Info("We can impersonate user for these namespaces:", impersonNamespaces)
	user.namespaces = impersonNamespaces
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

///check shared cache for shared namesapces if exists if not:
/// get all namepsaces using normal rest.config and cache those namespaces
/// for each namespace impersonate the user to get namesapces they have access to
/// cache those namespaces the user has access to within the user caches namespaces
