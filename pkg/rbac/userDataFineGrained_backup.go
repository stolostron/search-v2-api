// Copyright Contributors to the Open Cluster Management project
package rbac

/* Experimental code below. Remove before merging.

func (user *UserDataCache) buildClusterPersmissions(ctx context.Context) error {

	userPermissionList, err := user.getUserPermissions(ctx)
	if err != nil {
		klog.Error("Error getting UserPermissions. ", err)
		return err
	}

	// Using the new helper function to get the UserPermissionRules.
	userPermissionRules, err := GetSelfPermissionRules(ctx, userPermissionList, "list")
	if err != nil {
		klog.Error("Error getting UserPermissionRules. ", err)
	}
	klog.Info("UserPermissionRules from helper function:")
	for _, rule := range userPermissionRules {
		klog.Infof("  Rule: %+v", rule)
	}

	return nil
}

// https://github.com/stolostron/cluster-lifecycle-api/blob/main/helpers/userpermission/helpers.go

var adminResourceRule = authzv1.ResourceRule{
	Verbs:     []string{"*"},
	APIGroups: []string{"*"},
	Resources: []string{"*"},
}

// internal cache to keep permissionRule
type permissionRule struct {
	authzv1.ResourceRule
	cluster    string
	namespaces []string
}

func newPermissionRule(cluster string, namespaces []string, resourceRule authzv1.ResourceRule) *permissionRule {
	return &permissionRule{
		ResourceRule: resourceRule,
		cluster:      cluster,
		namespaces:   namespaces,
	}
}

func (p *permissionRule) key() string {
	shallow := *p
	shallow.cluster = ""
	data, _ := json.Marshal(shallow)
	h := fnv.New128a()
	h.Write(data)
	var sum [16]byte
	return fmt.Sprintf("%x", h.Sum(sum[:0]))
}

type PermissionRule struct {
	authzv1.ResourceRule
	Clusters   []string `json:"clusters"`
	Namespaces []string `json:"namespaces"`
}

// We use this to process a logic if a user is admin, all other
// permission rules are ignored.
type internalPermissionCache struct {
	// keeps a map of rules, the key is clusterName
	rulesMap map[string][]*permissionRule
}

func (i *internalPermissionCache) add(rules ...*permissionRule) {
	for _, rule := range rules {
		existing, ok := i.rulesMap[rule.cluster]
		if !ok {
			i.rulesMap[rule.cluster] = []*permissionRule{rule}
			continue
		}

		// if newly added rule is admin, override all the existing
		if equality.Semantic.DeepEqual(rule.ResourceRule, adminResourceRule) {
			i.rulesMap[rule.cluster] = []*permissionRule{rule}
			continue
		}

		var skipAdd bool
		for idx, existingRule := range existing {
			// if existing rule is admin, skip any update since it is the highest permission
			if equality.Semantic.DeepEqual(existingRule.ResourceRule, adminResourceRule) {
				skipAdd = true
				continue
			}

			// if resourceRule are the same, merge the namespace
			if equality.Semantic.DeepEqual(existingRule.ResourceRule, rule.ResourceRule) {
				namespaces := append(existingRule.namespaces, rule.namespaces...)
				i.rulesMap[rule.cluster][idx].namespaces = slices.Compact(namespaces)
				skipAdd = true
			}
		}

		if !skipAdd {
			i.rulesMap[rule.cluster] = append(existing, rule)
		}
	}
}

// consolidate all rules and return
func (i *internalPermissionCache) consolidateList() []PermissionRule {
	conslidatedMap := map[string]PermissionRule{}

	for _, clusterRules := range i.rulesMap {
		for _, rule := range clusterRules {
			// if two rules has the same namespaces and resourceRules, merge them to one.
			existing, ok := conslidatedMap[rule.key()]
			if !ok {
				conslidatedMap[rule.key()] = PermissionRule{
					ResourceRule: rule.ResourceRule,
					Clusters:     []string{rule.cluster},
					Namespaces:   rule.namespaces,
				}
			} else {
				existing.Clusters = append(existing.Clusters, rule.cluster)
				conslidatedMap[rule.key()] = existing
			}
		}
	}

	conslidatedList := []PermissionRule{}
	for _, rule := range conslidatedMap {
		conslidatedList = append(conslidatedList, rule)
	}
	return conslidatedList
}

// GetSelfPermissionRules retrieves and consolidates permission rules for the authenticated user across multiple clusters.
//
// This function queries all UserPermission resources accessible by the authenticated user and consolidates them into
// a list of PermissionRule objects. Each PermissionRule represents a set of RBAC permissions (verbs, API groups, resources)
// that apply to one or more clusters and namespaces.
//
// The consolidation process:
//   - Groups identical resource rules across multiple clusters into a single PermissionRule entry
//   - Merges namespace lists when the same resource rule exists within a single cluster
//   - Handles admin permissions (Verbs:*, APIGroups:*, Resources:*) specially by overriding other permissions for that cluster
//   - Filters rules by interested verbs if specified
//
// Parameters:
//   - ctx: Context for the API request
//   - config: Kubernetes REST config for the authenticated user whose permissions should be evaluated.
//     This must be the user's config, not a service account or admin config.
//   - interestedVerb: Optional list of verbs to filter by (e.g., "get", "list", "create").
//     If provided, only rules containing ALL specified verbs will be returned.
//     If the rule has wildcard verbs ("*"), it will always be included regardless of this filter.
//     If not provided, all permission rules are returned.
//
// Returns:
//   - []PermissionRule: A consolidated list of permission rules. Each rule contains:
//   - ResourceRule: The RBAC resource rule (Verbs, APIGroups, Resources)
//   - Clusters: List of cluster names where this rule applies
//   - Namespaces: List of namespaces where this rule applies (may include "*" for cluster-scoped)
//   - error: Any error encountered during the retrieval or processing
func GetSelfPermissionRules(ctx context.Context, userPermissions clusterviewv1alpha1.UserPermissionList, interestedVerb ...string) ([]PermissionRule, error) {
	// clusterViewClient, err := clusterviewclientset.NewForConfig(config)
	// if err != nil {
	// 	return nil, err
	// }

	// userPermissions, err := clusterViewClient.ClusterviewV1alpha1().UserPermissions().List(ctx, metav1.ListOptions{})
	// if err != nil {
	// 	return nil, err
	// }

	cache := &internalPermissionCache{
		rulesMap: make(map[string][]*permissionRule),
	}
	for _, userPermission := range userPermissions.Items {
		permissionRules := evaluateUserPermissionRule(userPermission, interestedVerb...)
		cache.add(permissionRules...)
	}
	return cache.consolidateList(), nil
}

func evaluateUserPermissionRule(userPermission clusterviewv1alpha1.UserPermission, interestedVerb ...string) []*permissionRule {
	// func evaluateUserPermissionRule(userPermission unstructured.Unstructured, interestedVerb ...string) []*permissionRule {
	var permissionRules []*permissionRule

	for _, rule := range userPermission.Status.ClusterRoleDefinition.Rules {
		// we conly care about * and filtered verbs
		haveSet := sets.New[string](rule.Verbs...)

		// If verbs in rule include "*", the output verb is "*"
		// If there are interested verbs, only return rules that has interested verbs included
		// return all verbs in the rule otherwise.
		var verbs []string
		if haveSet.HasAll("*") {
			verbs = rule.Verbs
		} else if len(interestedVerb) > 0 {
			if !haveSet.HasAll(interestedVerb...) {
				continue
			}
			verbs = interestedVerb
		} else {
			verbs = rule.Verbs
		}

		resourceRule := authzv1.ResourceRule{
			Verbs:     verbs,
			APIGroups: rule.APIGroups,
			Resources: rule.Resources,
		}
		for _, binding := range userPermission.Status.Bindings {
			p := newPermissionRule(binding.Cluster, binding.Namespaces, resourceRule)
			permissionRules = append(permissionRules, p)
		}
	}
	return permissionRules
}
*/
