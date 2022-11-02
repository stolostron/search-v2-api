package rbac

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	authv1 "k8s.io/api/authentication/v1"
	authz "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fake "k8s.io/client-go/kubernetes/fake"

	"k8s.io/client-go/rest"
	testingk8s "k8s.io/client-go/testing"
)

// Initialize cache object to use tests.
func mockNamespaceCache() *Cache {

	return &Cache{
		users:            map[string]*UserDataCache{},
		shared:           SharedData{},
		restConfig:       &rest.Config{},
		tokenReviews:     map[string]*tokenReviewCache{},
		tokenReviewsLock: sync.Mutex{},
	}
}

func setupToken(cache *Cache) *Cache {
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
func setupUserDataCache(cache *Cache, ud *UserDataCache) {
	cache.users["unique-user-id"] = ud
}

func addCSResources(cache *Cache, res []Resource) *Cache {
	if cache.shared.csResourcesMap == nil {
		cache.shared.csResourcesMap = map[Resource]struct{}{}
	}
	for _, resource := range res {
		cache.shared.csResourcesMap[resource] = struct{}{}
	}
	return cache
}

func Test_getNamespaces_emptyCache(t *testing.T) {
	mock_cache := mockNamespaceCache()
	mock_cache = setupToken(mock_cache)

	var namespaces []string

	mock_cache.shared.namespaces = append(namespaces, "some-namespace")

	rulesCheck := &authz.SelfSubjectRulesReview{
		Spec: authz.SelfSubjectRulesReviewSpec{
			Namespace: "some-namespace",
		},
		Status: authz.SubjectRulesReviewStatus{
			ResourceRules: []authz.ResourceRule{
				{
					Verbs:     []string{"list"},
					APIGroups: []string{"k8s.io"},
					Resources: []string{"nodes1", "nodes2"},
				},
			},
		},
	}
	fs := fake.Clientset{}
	fs.AddReactor("create", "selfsubjectaccessreviews", func(action testingk8s.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("Error creating ssar")
	})
	fs.AddReactor("create", "selfsubjectrulesreviews", func(action testingk8s.Action) (handled bool, ret runtime.Object, err error) {
		ret = action.(testingk8s.CreateAction).GetObject()
		_, ok := ret.(metav1.Object)
		if !ok {
			t.Error("Unexpected Error - expecting MetaObject with type *v1.SelfSubjectRulesReview")
			return
		}
		return true, rulesCheck, nil
	})
	ctx := context.WithValue(context.Background(), ContextAuthTokenKey, "123456")
	result, err := mock_cache.GetUserDataCache(ctx, fs.AuthorizationV1())

	if len(result.NsResources) != 1 ||
		result.NsResources["some-namespace"][0].Apigroup != "k8s.io" ||
		result.NsResources["some-namespace"][1].Apigroup != "k8s.io" ||
		result.NsResources["some-namespace"][0].Kind != "nodes1" ||
		result.NsResources["some-namespace"][1].Kind != "nodes2" {
		t.Errorf("Cache does not have expected namespace resources ")

	}
	if err != nil {
		t.Error("Unexpected error while obtaining namespaces.", err)
	}

}

func Test_getNamespaces_usingCache(t *testing.T) {
	var namespaces []string
	nsresources := make(map[string][]Resource)
	mock_cache := mockNamespaceCache()
	//mock cache for token review to get user data:
	mock_cache = setupToken(mock_cache)

	//mock cache for cluster-scoped resouces to get all namespaces:
	mock_cache.shared.namespaces = append(namespaces, "some-namespace")
	//mock cache for managed clusters
	managedclusters := map[string]struct{}{"some-namespace": {}, "some-nonmatching-namespace": {}}

	mock_cache.shared.managedClusters = managedclusters

	//mock cache for namespaced-resources:
	nsresources["some-namespace"] = append(nsresources["some-namespace"],
		Resource{Apigroup: "some-apigroup", Kind: "some-kind"})

	mock_cache.users["unique-user-id"] = &UserDataCache{
		UserData: UserData{ManagedClusters: managedclusters,
			NsResources: nsresources},
		csrCache:      cacheMetadata{updatedAt: time.Now()},
		nsrCache:      cacheMetadata{updatedAt: time.Now()},
		clustersCache: cacheMetadata{updatedAt: time.Now()},
	}

	rulesCheck := &authz.SelfSubjectRulesReview{
		Spec: authz.SelfSubjectRulesReviewSpec{
			Namespace: "some-namespace",
		},
		Status: authz.SubjectRulesReviewStatus{
			ResourceRules: []authz.ResourceRule{
				{
					Verbs:     []string{"list"},
					APIGroups: []string{"k8s.io"},
					Resources: []string{"nodes1", "nodes2"},
				},
			},
		},
	}
	fs := fake.NewSimpleClientset()
	fs.PrependReactor("create", "*", func(action testingk8s.Action) (handled bool, ret runtime.Object, err error) {
		ret = action.(testingk8s.CreateAction).GetObject()
		_, ok := ret.(metav1.Object)
		if !ok {
			t.Error("Unexpected Error - expecting MetaObject with type *v1.SelfSubjectRulesReview")
			return
		}
		return true, rulesCheck, nil
	})

	ctx := context.WithValue(context.Background(), ContextAuthTokenKey, "123456")
	result, err := mock_cache.GetUserDataCache(ctx, fs.AuthorizationV1())

	if len(result.NsResources) != 1 ||
		result.NsResources["some-nonmatching-namespace"] != nil ||
		result.NsResources["some-namespace"][0].Apigroup != "some-apigroup" ||
		result.NsResources["some-namespace"][0].Kind != "some-kind" {
		t.Error("Resources not in cache.")
	}
	if err != nil {
		t.Error("Unexpected error while obtaining namespaces.", err)
	}

}

func Test_getNamespaces_expiredCache(t *testing.T) {

	var namespaces []string
	nsresources := make(map[string][]Resource)

	mock_cache := mockNamespaceCache()

	//mock cache for token review to get user data:
	mock_cache = setupToken(mock_cache)
	mock_cache.shared.managedClusters = map[string]struct{}{"some-namespace": {}, "some-nonmatching-namespace": {}}

	//mock cache for cluster-scoped resouces to get all namespaces:
	mock_cache.shared.namespaces = append(namespaces, "some-namespace")

	//mock cache for namespaced-resources:
	nsresources["some-namespace"] = append(nsresources["some-namespace"],
		Resource{Apigroup: "some-apigroup", Kind: "some-kind"})

	last_cache_time := time.Now().Add(time.Duration(-5) * time.Minute)
	mock_cache.users["unique-user-id"] = &UserDataCache{
		UserData: UserData{NsResources: nsresources},
		nsrCache: cacheMetadata{updatedAt: last_cache_time},
	}
	rulesCheck := &authz.SelfSubjectRulesReview{
		Spec: authz.SelfSubjectRulesReviewSpec{
			Namespace: "some-namespace",
		},
		Status: authz.SubjectRulesReviewStatus{
			ResourceRules: []authz.ResourceRule{
				{
					Verbs:     []string{"list"},
					APIGroups: []string{"k8s.io"},
					Resources: []string{"nodes1", "nodes2"},
				},
			},
		},
	}
	fs := fake.Clientset{}
	fs.AddReactor("create", "selfsubjectaccessreviews", func(action testingk8s.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("Error creating ssar")
	})
	fs.AddReactor("create", "selfsubjectrulesreviews", func(action testingk8s.Action) (handled bool, ret runtime.Object, err error) {
		ret = action.(testingk8s.CreateAction).GetObject()
		_, ok := ret.(metav1.Object)
		if !ok {
			t.Error("Unexpected Error - expecting MetaObject with type *v1.SelfSubjectRulesReview")
			return
		}
		return true, rulesCheck, nil
	})
	ctx := context.WithValue(context.Background(), ContextAuthTokenKey, "123456")
	result, err := mock_cache.GetUserDataCache(ctx, fs.AuthorizationV1())

	if len(result.NsResources) != 1 || len(result.NsResources["some-namespace"]) != 2 ||
		result.NsResources["some-nonmatching-namespace"] != nil ||
		result.NsResources["some-namespace"][0].Apigroup != "k8s.io" ||
		result.NsResources["some-namespace"][1].Apigroup != "k8s.io" ||
		result.NsResources["some-namespace"][0].Kind != "nodes1" ||
		result.NsResources["some-namespace"][1].Kind != "nodes2" {
		t.Errorf("Cache does not have expected namespace resources ")

	}
	if err != nil {
		t.Error("Unexpected error while obtaining namespaces.", err)
	}

	//  Verify that cache was updated by checking the timestamp
	if !mock_cache.users["unique-user-id"].nsrCache.updatedAt.After(last_cache_time) {
		t.Error("Expected the cache.users.updatedAt to have a later timestamp")
	}
}

func Test_clusterScoped_usingCache(t *testing.T) {

	mock_cache := mockNamespaceCache()
	mock_cache = setupToken(mock_cache)
	res := []Resource{{Apigroup: "storage.k8s.io", Kind: "nodes"}}
	mock_cache = addCSResources(mock_cache, res)

	//mock cache for cluster-scoped resouces
	mock_cache.users["unique-user-id"] = &UserDataCache{
		UserData: UserData{
			CsResources:     []Resource{{Apigroup: "storage.k8s.io", Kind: "nodes"}},
			ManagedClusters: map[string]struct{}{"some-namespace": {}}},
		clustersCache: cacheMetadata{updatedAt: time.Now()},
		// Using current time , GetUserData should have the same values as cache
		csrCache: cacheMetadata{updatedAt: time.Now()},
		nsrCache: cacheMetadata{updatedAt: time.Now()},
	}
	ctx := context.WithValue(context.Background(), ContextAuthTokenKey, "123456")

	result, err := mock_cache.GetUserDataCache(ctx, nil)
	if len(result.CsResources) != 1 || result.CsResources[0].Kind != "nodes" || result.CsResources[0].Apigroup != "storage.k8s.io" {
		t.Error("Cluster scoped Resources not in user cache.")
	}
	if err != nil {
		t.Error("Unexpected error while obtaining cluster scoped resources.", err)
	}

}
func Test_clusterScoped_expiredCache(t *testing.T) {

	mock_cache := mockNamespaceCache()
	mock_cache = setupToken(mock_cache)

	//mock response objects for KUBEAPI call for SelfSubjectAccessReview
	trueCheck := &authz.SelfSubjectAccessReview{
		Spec: authz.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authz.ResourceAttributes{
				Verb:     "list",
				Group:    "storage.k8s.io",
				Resource: "nodes",
			},
		},
		Status: authz.SubjectAccessReviewStatus{
			Allowed: true,
		},
	}

	falseCheck := &authz.SelfSubjectAccessReview{
		Spec: authz.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authz.ResourceAttributes{
				Verb:     "list",
				Group:    "k8s.io",
				Resource: "csinodes",
			},
		},
		Status: authz.SubjectAccessReviewStatus{
			Allowed: false,
		},
	}

	fs := fake.NewSimpleClientset()
	fs.PrependReactor("create", "*", func(action testingk8s.Action) (handled bool, ret runtime.Object, err error) {
		ret = action.(testingk8s.CreateAction).GetObject()
		meta, ok := ret.(metav1.Object)
		if !ok {
			t.Error("Unexpected Error - expecting MetaObject with type *v1.SelfSubjectAccessReview")
			return
		}
		testSSAR := meta.(*authz.SelfSubjectAccessReview)
		// Mimic user has Authorization to list nodes
		if testSSAR.Spec.ResourceAttributes.Resource == "nodes" {
			return true, trueCheck, nil
		}
		// Mimic user has no Authorization to list other types
		return true, falseCheck, nil
	})

	res := []Resource{{Apigroup: "storage.k8s.io", Kind: "nodes"}, {Apigroup: "k8s.io", Kind: "csinodes"}}
	mock_cache = addCSResources(mock_cache, res)
	// Setup a allowed resource , we have have access to this resource through falseCheck Object
	// So when the next GetUserData executes user should not have this resource in allowed list
	last_cache_time := time.Now().Add(time.Duration(-5) * time.Minute)
	mock_cache.users["unique-user-id"] = &UserDataCache{
		UserData: UserData{
			CsResources: []Resource{{Apigroup: "k8s.io", Kind: "csinodes"}},
		},
		csrCache:    cacheMetadata{updatedAt: last_cache_time},
		authzClient: fs.AuthorizationV1(),
	}

	ctx := context.WithValue(context.Background(), ContextAuthTokenKey, "123456")
	result, err := mock_cache.GetUserDataCache(ctx, fs.AuthorizationV1())

	if len(result.CsResources) != 1 || result.CsResources[0].Kind != "nodes" {
		t.Error("Cluster scoped Resources not in user cache.")
		t.Errorf("Resource count present in cache %d", len(result.CsResources))
	}
	if err != nil {
		t.Error("Unexpected error while obtaining namespaces.", err)
	}

	//  Verify that cache was updated by checking the timestamp
	if !mock_cache.users["unique-user-id"].csrCache.updatedAt.After(last_cache_time) {
		t.Error("Expected the cache.users.updatedAt to have a later timestamp")
	}

}
func Test_managedClusters_emptyCache(t *testing.T) {

	mock_cache := mockNamespaceCache()
	mock_cache = setupToken(mock_cache)

	var namespaces []string
	mock_cache.shared.managedClusters = map[string]struct{}{"some-managed-cluster": {}, "some-managed-cluster1": {}}

	mock_cache.shared.namespaces = append(namespaces, "some-managed-cluster", "some-managed-cluster1")

	//mock response objects for KUBEAPI call for SelfSubjectRulesReview
	createRule := &authz.SelfSubjectRulesReview{
		Spec: authz.SelfSubjectRulesReviewSpec{
			Namespace: "some-managed-cluster",
		},
		Status: authz.SubjectRulesReviewStatus{
			ResourceRules: []authz.ResourceRule{
				{
					Verbs:     []string{"create"},
					APIGroups: []string{"apigroup"},
					Resources: []string{"managedclusterviews"},
				},
			},
		},
	}
	notCreateRule := &authz.SelfSubjectRulesReview{
		Spec: authz.SelfSubjectRulesReviewSpec{
			Namespace: "some-managed-cluster1",
		},
		Status: authz.SubjectRulesReviewStatus{
			ResourceRules: []authz.ResourceRule{
				{
					Verbs:     []string{"list"},
					APIGroups: []string{"apigroup"},
					Resources: []string{"managedclusterviews"},
				},
			},
		},
	}

	fs := fake.Clientset{}
	fs.AddReactor("create", "selfsubjectaccessreviews", func(action testingk8s.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("Error creating ssar")
	})
	fs.AddReactor("create", "selfsubjectrulesreviews", func(action testingk8s.Action) (handled bool, ret runtime.Object, err error) {
		ret = action.(testingk8s.CreateAction).GetObject()
		meta, ok := ret.(metav1.Object)
		if !ok {
			t.Error("Unexpected Error - expecting MetaObject with type *v1.SelfSubjectRulesReview")
			return
		}
		testSSRR := meta.(*authz.SelfSubjectRulesReview)
		//	Mimic user has Authorization to create managedclusterview
		if testSSRR.Spec.Namespace == "some-managed-cluster" {
			return true, createRule, nil
		}

		// // Mimic user has no Authorization to create managedclusterview
		return true, notCreateRule, nil

	})
	ctx := context.WithValue(context.Background(), ContextAuthTokenKey, "123456")
	result, err := mock_cache.GetUserDataCache(ctx, fs.AuthorizationV1())

	_, ok := result.ManagedClusters["some-managed-cluster"]
	if len(result.ManagedClusters) != 1 || !ok {
		t.Errorf("Managed cluster count present in cache %d", len(result.ManagedClusters))
	}
	if err != nil {
		t.Error("Unexpected error while obtaining namespaces.", err)
	}

}

func Test_managedClusters_usingCache(t *testing.T) {

	mock_cache := mockNamespaceCache()
	mock_cache = setupToken(mock_cache)

	res := []Resource{{Apigroup: "storage.k8s.io", Kind: "nodes"}}
	mock_cache = addCSResources(mock_cache, res)

	//mock cache for cluster-scoped resouces
	mock_cache.users["unique-user-id"] = &UserDataCache{
		UserData: UserData{
			CsResources:     []Resource{{Apigroup: "storage.k8s.io", Kind: "nodes"}},
			ManagedClusters: map[string]struct{}{"some-managed-cluster": {}, "some-other-managed-cluster": {}},
		},
		clustersCache: cacheMetadata{updatedAt: time.Now()},
		// Using current time , GetUserData should have the same values as cache
		csrCache: cacheMetadata{updatedAt: time.Now()},
		nsrCache: cacheMetadata{updatedAt: time.Now()},
	}
	ctx := context.WithValue(context.Background(), ContextAuthTokenKey, "123456")

	result, err := mock_cache.GetUserDataCache(ctx, nil)
	_, mc1Present := result.ManagedClusters["some-managed-cluster"]
	_, mc2Present := result.ManagedClusters["some-other-managed-cluster"]

	if len(result.CsResources) != 1 || result.CsResources[0].Kind != "nodes" || result.CsResources[0].Apigroup != "storage.k8s.io" ||
		!mc1Present || !mc2Present {
		t.Error("Cluster scoped Resources not in user cache.")
	}
	if err != nil {
		t.Error("Unexpected error while obtaining cluster scoped resources.", err)
	}

}

func Test_managedCluster_expiredCache(t *testing.T) {
	mock_cache := mockNamespaceCache()
	mock_cache = setupToken(mock_cache)

	managedClusters := map[string]struct{}{"some-managed-cluster": {}, "some-other-managed-cluster": {}}

	// mock clusters in user cache
	var namespaces []string
	//mock mc from shared cache
	mock_cache.shared.managedClusters = managedClusters
	mock_cache.shared.namespaces = append(namespaces, "some-managed-cluster", "some-managed-cluster1")

	//mock response objects for KUBEAPI call for SelfSubjectRulesReview
	createRule := &authz.SelfSubjectRulesReview{
		Spec: authz.SelfSubjectRulesReviewSpec{
			Namespace: "some-managed-cluster",
		},
		Status: authz.SubjectRulesReviewStatus{
			ResourceRules: []authz.ResourceRule{
				{
					Verbs:     []string{"create"},
					APIGroups: []string{"apigroup"},
					Resources: []string{"managedclusterviews"},
				},
			},
		},
	}
	notCreateRule := &authz.SelfSubjectRulesReview{
		Spec: authz.SelfSubjectRulesReviewSpec{
			Namespace: "some-managed-cluster1",
		},
		Status: authz.SubjectRulesReviewStatus{
			ResourceRules: []authz.ResourceRule{
				{
					Verbs:     []string{"list"},
					APIGroups: []string{"apigroup"},
					Resources: []string{"managedclusterviews"},
				},
			},
		},
	}

	fs := fake.Clientset{}
	fs.AddReactor("create", "selfsubjectaccessreviews", func(action testingk8s.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("Error creating ssar")
	})
	fs.AddReactor("create", "selfsubjectrulesreviews", func(action testingk8s.Action) (handled bool, ret runtime.Object, err error) {
		ret = action.(testingk8s.CreateAction).GetObject()
		meta, ok := ret.(metav1.Object)
		if !ok {
			t.Error("Unexpected Error - expecting MetaObject with type *v1.SelfSubjectRulesReview")
			return
		}
		testSSRR := meta.(*authz.SelfSubjectRulesReview)
		//	Mimic user has Authorization to create managedclusterview
		if testSSRR.Spec.Namespace == "some-managed-cluster" {
			return true, createRule, nil
		}

		// // Mimic user has no Authorization to create managedclusterview
		return true, notCreateRule, nil

	})

	pastManClusters := map[string]struct{}{}
	pastManClusters["past-managed-cluster"] = struct{}{}

	last_cache_time := time.Now().Add(time.Duration(-5) * time.Minute)
	mock_cache.users["unique-user-id"] = &UserDataCache{
		UserData:      UserData{ManagedClusters: pastManClusters},
		clustersCache: cacheMetadata{updatedAt: last_cache_time},
		authzClient:   fs.AuthorizationV1(),
	}

	ctx := context.WithValue(context.Background(), ContextAuthTokenKey, "123456")
	result, err := mock_cache.GetUserDataCache(ctx, fs.AuthorizationV1())
	_, ok := result.ManagedClusters["some-managed-cluster"]
	if len(result.ManagedClusters) != 1 || !ok {
		t.Errorf("Managed cluster count present in cache %d", len(result.ManagedClusters))
	}
	if err != nil {
		t.Error("Unexpected error while obtaining namespaces.", err)
	}

	//  Verify that cache was updated by checking the timestamp
	if !mock_cache.users["unique-user-id"].clustersCache.updatedAt.After(last_cache_time) {
		t.Error("Expected the cache.users.updatedAt to have a later timestamp")
	}

}

func Test_managedCluster_GetUserData(t *testing.T) {
	mock_cache := mockNamespaceCache()
	mock_cache = setupToken(mock_cache)

	managedClusters := map[string]struct{}{"managed-cluster1": {}}

	csRes := []Resource{{Kind: "kind1", Apigroup: ""}, {Kind: "kind2", Apigroup: "v1"}}
	nsRes := make(map[string][]Resource)
	nsRes["ns1"] = []Resource{{Kind: "kind1", Apigroup: ""}, {Kind: "kind2", Apigroup: "v1"}}
	nsRes["ns2"] = []Resource{{Kind: "kind3", Apigroup: ""}, {Kind: "kind4", Apigroup: "v1"}}

	last_cache_time := time.Now().Add(time.Duration(-5) * time.Minute)
	mock_cache.users["unique-user-id"] = &UserDataCache{
		UserData:      UserData{ManagedClusters: managedClusters, CsResources: csRes, NsResources: nsRes},
		clustersCache: cacheMetadata{updatedAt: last_cache_time},
	}
	csResResult := mock_cache.users["unique-user-id"].GetCsResources()
	if len(csResResult) != 2 {
		t.Errorf("Expected 2 clusterScoped resources but got %d", len(csResResult))

	}
	nsResResult := mock_cache.users["unique-user-id"].GetNsResources()
	if len(nsResResult) != 2 {
		t.Errorf("Expected 2 namespace Scoped resources but got %d", len(nsResResult))
	}
	mcResult := mock_cache.users["unique-user-id"].GetManagedClusters()
	if len(mcResult) != 1 {
		t.Errorf("Expected 1 managed cluster but got %d", len(mcResult))
	}
}

func Test_getUserData(t *testing.T) {

	mock_cache := mockNamespaceCache()
	mock_cache = setupToken(mock_cache)
	mock_cache.users["unique-user-id"] = &UserDataCache{
		UserData: UserData{
			CsResources:     []Resource{{Apigroup: "storage.k8s.io", Kind: "nodes"}},
			ManagedClusters: map[string]struct{}{"some-managed-cluster": {}, "some-other-managed-cluster": {}},
			NsResources:     map[string][]Resource{"ns1": {{Apigroup: "", Kind: "pods"}}},
		},
		// Using current time , GetUserData should have the same values as cache
		clustersCache: cacheMetadata{updatedAt: time.Now()},
		csrCache:      cacheMetadata{updatedAt: time.Now()},
		nsrCache:      cacheMetadata{updatedAt: time.Now()},
	}
	ctx := context.WithValue(context.Background(), ContextAuthTokenKey, "123456")

	result, err := mock_cache.GetUserData(ctx)

	if err != nil {
		t.Error("Unexpected error while getting userdata.", err)
	}

	if len(result.CsResources) != 1 {
		t.Errorf("Expected 1 clusterScoped resources but got %d", len(result.CsResources))

	}
	if len(result.NsResources) != 1 {
		t.Errorf("Expected 1 namespace Scoped resource but got %d", len(result.NsResources))
	}
	if len(result.ManagedClusters) != 2 {
		t.Errorf("Expected 2 managed clusters but got %d", len(result.ManagedClusters))
	}
}

func Test_setImpersonationUserInfo(t *testing.T) {

	ui := authv1.UserInfo{
		Username: "test-user",
		UID:      "12345",
		Groups:   []string{"group1"},
		Extra: map[string]authv1.ExtraValue{
			"extraKey": []string{"extraValue"}},
	}

	impConf := setImpersonationUserInfo(ui)
	assert.Equal(t, ui.UID, impConf.UID)
	assert.Equal(t, ui.Username, impConf.UserName)
	assert.Equal(t, ui.Groups, impConf.Groups)
	assert.Equal(t, len(ui.Extra), len(impConf.Extra))
}

func Test_getImpersonationClientSet(t *testing.T) {
	mock_cache := mockNamespaceCache()
	mock_cache = setupToken(mock_cache)

	udc := &UserDataCache{
		UserData: UserData{},
		nsrCache: cacheMetadata{updatedAt: time.Now()},
	}
	_, err := udc.getImpersonationClientSet("123456", mock_cache)
	// Ensure that there is no error
	assert.Nil(t, err)

}

func Test_hasAccessToAllResourcesInNamespace(t *testing.T) {
	mock_cache := mockNamespaceCache()
	mock_cache = setupToken(mock_cache)

	var namespaces []string

	mock_cache.shared.namespaces = append(namespaces, "some-namespace")

	rulesCheck := &authz.SelfSubjectRulesReview{
		Spec: authz.SelfSubjectRulesReviewSpec{
			Namespace: "some-namespace",
		},
		Status: authz.SubjectRulesReviewStatus{
			ResourceRules: []authz.ResourceRule{
				{
					Verbs:     []string{"list"},
					APIGroups: []string{"v1", "*"},
					Resources: []string{"pods", "*"},
				},
			},
		},
	}
	fs := fake.Clientset{}
	fs.AddReactor("create", "selfsubjectaccessreviews", func(action testingk8s.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("Error creating ssar")
	})
	fs.AddReactor("create", "selfsubjectrulesreviews", func(action testingk8s.Action) (handled bool, ret runtime.Object, err error) {
		ret = action.(testingk8s.CreateAction).GetObject()
		_, ok := ret.(metav1.Object)
		if !ok {
			t.Error("Unexpected Error - expecting MetaObject with type *v1.SelfSubjectRulesReview")
			return
		}
		return true, rulesCheck, nil
	})
	ctx := context.WithValue(context.Background(), ContextAuthTokenKey, "123456")
	result, err := mock_cache.GetUserDataCache(ctx, fs.AuthorizationV1())

	if len(result.NsResources) != 1 ||
		result.NsResources["some-namespace"][0].Apigroup != "*" ||
		result.NsResources["some-namespace"][0].Kind != "*" {
		t.Errorf("Cache does not have expected namespace resources ")

	}
	if err != nil {
		t.Error("Unexpected error while obtaining namespaces.", err)
	}

}

func Test_hasAccessToAllResources(t *testing.T) {
	mock_cache := mockNamespaceCache()
	mock_cache = setupToken(mock_cache)

	var namespaces []string

	mock_cache.shared.namespaces = append(namespaces, "some-namespace")
	mock_cache.shared.managedClusters = map[string]struct{}{"managed-cluster1": {}}

	accessCheck := &authz.SelfSubjectAccessReview{
		Spec: authz.SelfSubjectAccessReviewSpec{},
		Status: authz.SubjectAccessReviewStatus{
			Allowed: true,
			Denied:  false,
			Reason:  "Service account allows it",
		},
	}
	fs := fake.Clientset{}
	fs.AddReactor("create", "selfsubjectaccessreviews", func(action testingk8s.Action) (handled bool, ret runtime.Object, err error) {
		return true, accessCheck, nil
	})

	ctx := context.WithValue(context.Background(), ContextAuthTokenKey, "123456")
	result, err := mock_cache.GetUserDataCache(ctx, fs.AuthorizationV1())

	if len(result.NsResources) != 1 ||
		result.NsResources["*"][0].Apigroup != "*" ||
		result.NsResources["*"][0].Kind != "*" {
		t.Errorf("Cache does not have expected namespace resources ")

	}
	if len(result.CsResources) != 1 ||
		result.CsResources[0].Apigroup != "*" ||
		result.CsResources[0].Kind != "*" {
		t.Errorf("Cache does not have expected cluster-scoped resources ")

	}
	_, mcPresent := result.ManagedClusters["managed-cluster1"]
	if len(result.ManagedClusters) != 1 ||
		!mcPresent {
		t.Errorf("Cache does not have expected managed cluster resources ")

	}
	if err != nil {
		t.Error("Unexpected error while obtaining SSAR.", err)
	}
}

//User should have access to ManagedClusters
func Test_updateUserManagedClusterList(t *testing.T) {
	mock_cache := mockNamespaceCache()
	mock_cache = setupToken(mock_cache)

	udc := &UserDataCache{
		UserData: UserData{ManagedClusters: make(map[string]struct{})},
		nsrCache: cacheMetadata{updatedAt: time.Now()},
	}
	// All namespaces list
	namespaces := map[string]struct{}{"some-namespace": {}, "some-nonmatching-namespace": {}, "invalid-namespace": {}}
	managedclusters := map[string]struct{}{"some-namespace": {}, "some-nonmatching-namespace": {}}
	//mock cache for managed clusters
	mock_cache.shared.managedClusters = managedclusters

	for ns := range namespaces {
		udc.updateUserManagedClusterList(mock_cache, ns)
	}
	assert.Equal(t, len(managedclusters), len(udc.ManagedClusters))
}
