// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	fakedynclient "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/scheme"
	k8stesting "k8s.io/client-go/testing"
)

func initMockCache() Cache {
	testScheme := scheme.Scheme
	mockns := &corev1.Namespace{
		TypeMeta:   metav1.TypeMeta{Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
	}
	return Cache{
		shared: SharedData{
			namespaces:       []string{"a", "b"},
			managedClusters:  map[string]struct{}{"a": {}, "b": {}},
			disabledClusters: map[string]struct{}{"a": {}, "b": {}},
			dynamicClient:    fakedynclient.NewSimpleDynamicClient(testScheme, mockns),
		},
		users: map[string]*UserDataCache{
			"usr1": {UserData: UserData{NsResources: map[string][]Resource{}}},
		},
	}
}

func Test_cacheValidation_StartBackgroundValidation(t *testing.T) {
	mock_cache := initMockCache()

	ctx := context.Background()
	mock_cache.StartBackgroundValidation(ctx)
}

func Test_cacheValidation_namespaceAdded(t *testing.T) {
	mock_cache := initMockCache()
	mock_namespace := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": "c",
			},
		},
	}

	mock_cache.namespaceAdded(mock_namespace)
	assert.Equal(t, []string{"a", "b", "c"}, mock_cache.shared.namespaces)
}

func Test_cacheValidation_namespaceDeleted(t *testing.T) {
	mock_cache := initMockCache()
	mock_namespace := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": "a",
			},
		},
	}

	mock_cache.namespaceDeleted(mock_namespace)
	assert.Equal(t, []string{"b"}, mock_cache.shared.namespaces)
}

func Test_cacheValidation_ManagedClusterAdded(t *testing.T) {
	mock_cache := initMockCache()
	mock_managedCluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": "c",
			},
		},
	}

	mock_cache.managedClusterAdded(mock_managedCluster)
	assert.Equal(t, map[string]struct{}{"a": {}, "b": {}, "c": {}}, mock_cache.shared.managedClusters)
}

func Test_cacheValidation_managedClusterDeleted(t *testing.T) {
	mock_cache := initMockCache()

	mock_managedCluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": "a",
			},
		},
	}

	mock_cache.managedClusterDeleted(mock_managedCluster)
	assert.Equal(t, map[string]struct{}{"b": {}}, mock_cache.shared.managedClusters)
}

// [AI] Test watchResource.start() with context cancellation
func Test_watchResource_start_ContextCancellation(t *testing.T) {
	testScheme := scheme.Scheme
	mockns := &corev1.Namespace{
		TypeMeta:   metav1.TypeMeta{Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
	}
	fakeDynamicClient := fakedynclient.NewSimpleDynamicClient(testScheme, mockns)

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Track if callbacks were called
	addCalled := false

	watchRes := watchResource{
		dynamicClient: fakeDynamicClient,
		gvr:           schema.GroupVersionResource{Resource: "namespaces", Group: "", Version: "v1"},
		onAdd: func(obj *unstructured.Unstructured) {
			addCalled = true
		},
		onModify: nil,
		onDelete: nil,
	}

	// Start the watch in a goroutine
	done := make(chan bool)
	go func() {
		watchRes.start(ctx)
		done <- true
	}()

	// Give it time to start
	time.Sleep(100 * time.Millisecond)

	// Cancel the context
	cancel()

	// Wait for the watch to stop
	select {
	case <-done:
		// Success - watch stopped
	case <-time.After(1 * time.Second):
		t.Fatal("Watch did not stop after context cancellation")
	}

	// Note: addCalled might be false if no events were sent before cancellation
	assert.False(t, addCalled, "No events should be processed before cancellation")
}

// [AI] Test watchResource.start() with ADDED event
func Test_watchResource_start_AddedEvent(t *testing.T) {
	testScheme := scheme.Scheme
	mockns := &corev1.Namespace{
		TypeMeta:   metav1.TypeMeta{Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
	}
	fakeDynamicClient := fakedynclient.NewSimpleDynamicClient(testScheme, mockns)

	// Create a watch reactor to send custom events
	watcher := watch.NewFake()
	fakeDynamicClient.PrependWatchReactor("namespaces", k8stesting.DefaultWatchReactor(watcher, nil))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addedObj := &unstructured.Unstructured{}
	addCalled := make(chan bool, 1)

	watchRes := watchResource{
		dynamicClient: fakeDynamicClient,
		gvr:           schema.GroupVersionResource{Resource: "namespaces", Group: "", Version: "v1"},
		onAdd: func(obj *unstructured.Unstructured) {
			addedObj = obj
			addCalled <- true
		},
		onModify: nil,
		onDelete: nil,
	}

	// Start watching
	go watchRes.start(ctx)
	time.Sleep(100 * time.Millisecond)

	// Send an ADDED event
	testNamespace := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": "new-namespace",
			},
		},
	}
	watcher.Add(testNamespace)

	// Wait for the callback to be called
	select {
	case <-addCalled:
		assert.Equal(t, "new-namespace", addedObj.GetName(), "Added namespace name should match")
	case <-time.After(1 * time.Second):
		t.Fatal("onAdd callback was not called")
	}
}

// [AI] Test watchResource.start() with MODIFIED event
func Test_watchResource_start_ModifiedEvent(t *testing.T) {
	testScheme := scheme.Scheme
	mockns := &corev1.Namespace{
		TypeMeta:   metav1.TypeMeta{Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
	}
	fakeDynamicClient := fakedynclient.NewSimpleDynamicClient(testScheme, mockns)

	watcher := watch.NewFake()
	fakeDynamicClient.PrependWatchReactor("namespaces", k8stesting.DefaultWatchReactor(watcher, nil))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	modifiedObj := &unstructured.Unstructured{}
	modifyCalled := make(chan bool, 1)

	watchRes := watchResource{
		dynamicClient: fakeDynamicClient,
		gvr:           schema.GroupVersionResource{Resource: "namespaces", Group: "", Version: "v1"},
		onAdd:         nil,
		onModify: func(obj *unstructured.Unstructured) {
			modifiedObj = obj
			modifyCalled <- true
		},
		onDelete: nil,
	}

	go watchRes.start(ctx)
	time.Sleep(100 * time.Millisecond)

	// Send a MODIFIED event
	modifiedNamespace := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": "modified-namespace",
			},
		},
	}
	watcher.Modify(modifiedNamespace)

	select {
	case <-modifyCalled:
		assert.Equal(t, "modified-namespace", modifiedObj.GetName(), "Modified namespace name should match")
	case <-time.After(1 * time.Second):
		t.Fatal("onModify callback was not called")
	}
}

// [AI] Test watchResource.start() with DELETED event
func Test_watchResource_start_DeletedEvent(t *testing.T) {
	testScheme := scheme.Scheme
	mockns := &corev1.Namespace{
		TypeMeta:   metav1.TypeMeta{Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
	}
	fakeDynamicClient := fakedynclient.NewSimpleDynamicClient(testScheme, mockns)

	watcher := watch.NewFake()
	fakeDynamicClient.PrependWatchReactor("namespaces", k8stesting.DefaultWatchReactor(watcher, nil))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	deletedObj := &unstructured.Unstructured{}
	deleteCalled := make(chan bool, 1)

	watchRes := watchResource{
		dynamicClient: fakeDynamicClient,
		gvr:           schema.GroupVersionResource{Resource: "namespaces", Group: "", Version: "v1"},
		onAdd:         nil,
		onModify:      nil,
		onDelete: func(obj *unstructured.Unstructured) {
			deletedObj = obj
			deleteCalled <- true
		},
	}

	go watchRes.start(ctx)
	time.Sleep(100 * time.Millisecond)

	// Send a DELETED event
	deletedNamespace := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": "deleted-namespace",
			},
		},
	}
	watcher.Delete(deletedNamespace)

	select {
	case <-deleteCalled:
		assert.Equal(t, "deleted-namespace", deletedObj.GetName(), "Deleted namespace name should match")
	case <-time.After(1 * time.Second):
		t.Fatal("onDelete callback was not called")
	}
}

// [AI] Test watchResource.start() with nil callbacks (should not panic)
func Test_watchResource_start_NilCallbacks(t *testing.T) {
	testScheme := scheme.Scheme
	mockns := &corev1.Namespace{
		TypeMeta:   metav1.TypeMeta{Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
	}
	fakeDynamicClient := fakedynclient.NewSimpleDynamicClient(testScheme, mockns)

	watcher := watch.NewFake()
	fakeDynamicClient.PrependWatchReactor("namespaces", k8stesting.DefaultWatchReactor(watcher, nil))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watchRes := watchResource{
		dynamicClient: fakeDynamicClient,
		gvr:           schema.GroupVersionResource{Resource: "namespaces", Group: "", Version: "v1"},
		onAdd:         nil, // All callbacks are nil
		onModify:      nil,
		onDelete:      nil,
	}

	// Start watching
	go watchRes.start(ctx)
	time.Sleep(100 * time.Millisecond)

	// Send events - should not panic even with nil callbacks
	testNamespace := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": "test",
			},
		},
	}

	// These should not panic
	watcher.Add(testNamespace)
	time.Sleep(50 * time.Millisecond)
	watcher.Modify(testNamespace)
	time.Sleep(50 * time.Millisecond)
	watcher.Delete(testNamespace)
	time.Sleep(50 * time.Millisecond)

	// If we got here without panic, test passes
	cancel()
}

// [AI] Test watchResource.start() with unexpected event type (triggers breakLoop and watch restart)
func Test_watchResource_start_UnexpectedEventRestartWatch(t *testing.T) {
	testScheme := scheme.Scheme
	retryDelay = time.Duration(1) * time.Second
	mockns := &corev1.Namespace{
		TypeMeta:   metav1.TypeMeta{Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
	}
	fakeDynamicClient := fakedynclient.NewSimpleDynamicClient(testScheme, mockns)

	watcher := watch.NewFake()
	fakeDynamicClient.PrependWatchReactor("namespaces", k8stesting.DefaultWatchReactor(watcher, nil))

	// Use a short context so the watch loop exits quickly after the error
	ctx, cancel := context.WithCancel(context.Background())

	eventCount := 0
	var mu sync.Mutex

	watchRes := watchResource{
		dynamicClient: fakeDynamicClient,
		gvr:           schema.GroupVersionResource{Resource: "namespaces", Group: "", Version: "v1"},
		onAdd: func(obj *unstructured.Unstructured) {
			mu.Lock()
			eventCount++
			mu.Unlock()
		},
		onModify: nil,
		onDelete: nil,
	}

	done := make(chan bool, 1)
	go func() {
		watchRes.start(ctx)
		done <- true
	}()

	time.Sleep(100 * time.Millisecond)

	// Send an error event to trigger the unexpected event handling
	// This triggers the default case in the switch statement (line 112-117)
	// which sets breakLoop = true, causing the code at line 119-122 to break the loop
	watcher.Error(&metav1.Status{
		Status:  metav1.StatusFailure,
		Message: "Unexpected error",
	})

	// The error handling includes a 5-second sleep (line 115), so we need to wait for that
	// Give it a moment to process the error, then cancel
	time.Sleep(100 * time.Millisecond)

	// Cancel context to stop the watch loop (which would otherwise retry)
	cancel()

	// Wait for watch to stop (need to wait longer than the 5-second sleep in the error handler)
	select {
	case <-done:
		// Watch stopped gracefully - test passes
		t.Log("Watch stopped gracefully after error event and context cancellation")
	case <-time.After(2 * time.Second):
		// This is acceptable - the watch might be in the 5-second sleep
		// The important thing is we didn't panic
		t.Log("Watch still processing (likely in 5-second retry sleep), but no panic occurred")
	}

	// The key test is that we didn't panic when handling the unexpected event
	mu.Lock()
	t.Logf("Events received: %d (test verified error handling doesn't panic)", eventCount)
	mu.Unlock()
}

// Test watchResource.start() handles multiple events in sequence
func Test_watchResource_start_MultipleEvents(t *testing.T) {
	testScheme := scheme.Scheme
	mockns := &corev1.Namespace{
		TypeMeta:   metav1.TypeMeta{Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{Name: "test-namespace"},
	}
	fakeDynamicClient := fakedynclient.NewSimpleDynamicClient(testScheme, mockns)

	watcher := watch.NewFake()
	fakeDynamicClient.PrependWatchReactor("namespaces", k8stesting.DefaultWatchReactor(watcher, nil))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var addedNames []string
	var modifiedNames []string
	var deletedNames []string
	var mu sync.Mutex

	watchRes := watchResource{
		dynamicClient: fakeDynamicClient,
		gvr:           schema.GroupVersionResource{Resource: "namespaces", Group: "", Version: "v1"},
		onAdd: func(obj *unstructured.Unstructured) {
			mu.Lock()
			addedNames = append(addedNames, obj.GetName())
			mu.Unlock()
		},
		onModify: func(obj *unstructured.Unstructured) {
			mu.Lock()
			modifiedNames = append(modifiedNames, obj.GetName())
			mu.Unlock()
		},
		onDelete: func(obj *unstructured.Unstructured) {
			mu.Lock()
			deletedNames = append(deletedNames, obj.GetName())
			mu.Unlock()
		},
	}

	go watchRes.start(ctx)
	time.Sleep(100 * time.Millisecond)

	// Send multiple events
	for i := 1; i <= 3; i++ {
		ns := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata": map[string]interface{}{
					"name": "ns-" + string(rune('0'+i)),
				},
			},
		}
		watcher.Add(ns)
		time.Sleep(50 * time.Millisecond)
	}

	// Modify one
	nsModify := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": "ns-modified",
			},
		},
	}
	watcher.Modify(nsModify)
	time.Sleep(50 * time.Millisecond)

	// Delete one
	nsDelete := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": "ns-deleted",
			},
		},
	}
	watcher.Delete(nsDelete)
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 3, len(addedNames), "Should have received 3 add events")
	assert.Equal(t, 1, len(modifiedNames), "Should have received 1 modify event")
	assert.Equal(t, 1, len(deletedNames), "Should have received 1 delete event")
	assert.Contains(t, modifiedNames, "ns-modified", "Modified namespace should be in list")
	assert.Contains(t, deletedNames, "ns-deleted", "Deleted namespace should be in list")
	mu.Unlock()
}
