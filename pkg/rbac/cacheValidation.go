// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
)

func StartCacheValidation(ctx context.Context) {
	for {
		cacheInst.backgroundValidation(ctx)
		klog.Warning("Cache background validation stopped. Restarting process.")
		time.Sleep(1 * time.Second)
	}
}

func (c *Cache) backgroundValidation(ctx context.Context) {
	klog.Info("Starting cache background validation.")

	// Watch namespaces
	namespacesGVR := schema.GroupVersionResource{Resource: "namespaces", Group: "", Version: "v1"}
	c.watch(ctx, namespacesGVR)
}

// Watch resource
func (c *Cache) watch(ctx context.Context, gvr schema.GroupVersionResource) {
	watch, watchError := c.dynamicClient.Resource(gvr).Watch(ctx, metav1.ListOptions{})
	if watchError != nil {
		klog.Warningf("Error watching %s.  Error: %s", gvr.String(), watchError)
		return
	}
	defer watch.Stop()

	klog.V(3).Infof("Watching\t[Group: %s \tKind: %s]", gvr.Group, gvr.Resource)

	for {
		select {
		case <-ctx.Done():
			klog.V(2).Info("Informer watch() was stopped. ", gvr.String())
			return

		case event := <-watch.ResultChan(): // Read events from the watch channel.
			//  Process ADDED or DELETED, events.
			o, error := runtime.UnstructuredConverter.ToUnstructured(runtime.DefaultUnstructuredConverter, &event.Object)
			if error != nil {
				klog.Warningf("Error converting %s event.Object to unstructured.Unstructured on ADDED event. %s",
					gvr.Resource, error)
			}
			obj := &unstructured.Unstructured{Object: o}

			switch event.Type {
			case "ADDED":
				// klog.V(3).Infof("Received ADDED event. Kind: %s ", gvr.Resource)
				klog.Infof("Event: ADDED  Resource: %s  Name: %s", gvr.Resource, obj.GetName())
				c.shared.addNamespace(obj.GetName())

			case "DELETED":
				// klog.V(3).Infof("Received DELETED event. Kind: %s ", gvr.Resource)
				klog.Infof("Event: DELETED  Resource: %s  Name: %s", gvr.Resource, obj.GetName())
				c.shared.deleteNamespace(obj.GetName())

			case "MODIFIED":
				// klog.V(3).Infof("Received MODIFY event. Kind: %s ", gvr.Resource)
				break

			default:
				klog.V(2).Infof("Received unexpected event. Ending listAndWatch() for %s", gvr.String())
				return
			}
		}
	}
}

func (shared *SharedData) addNamespace(ns string) {
	shared.nsLock.Lock()
	defer shared.nsLock.Unlock()
	shared.namespaces = append(shared.namespaces, ns)
	shared.nsUpdatedAt = time.Now()

}

func (shared *SharedData) deleteNamespace(ns string) {
	shared.nsLock.Lock()
	defer shared.nsLock.Unlock()
	newNsamespaces := make([]string, 0)
	for _, n := range shared.namespaces {
		if n != ns {
			newNsamespaces = append(newNsamespaces, n)
		}
	}
	shared.namespaces = newNsamespaces
	shared.nsUpdatedAt = time.Now()
}
