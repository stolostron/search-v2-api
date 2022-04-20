//
// Copyright Contributors to the Open Cluster Management project
package rbac

import (
	"time"
)

type rule struct {
	// Action is always List.
	apigroup     string
	kind         string
	allowedNames []string // resourceNames: If empty all resources are allowed.
}

type UserRbac struct {
	// Identify the user
	id    string
	token string

	// Track user activity
	firstActive   time.Time
	lastActive    time.Time
	lastValidated time.Time

	// Cache Authorization Rules
	namespaces              []string
	clusterResourceRules    []rule
	namespacedResourceRules map[string]rule

	// Track validation
	needUpdate bool
}
