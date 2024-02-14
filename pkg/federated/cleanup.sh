# !/bin/bash
# Copyright Contributors to the Open Cluster Management project

# Delete resources created by setup.sh
MANAGED_HUBS=($(oc get managedcluster -o json | jq -r '.items[] | select(.status.clusterClaims[] | .name == "hub.open-cluster-management.io" and .value != "NotInstalled") | .metadata.name'))

for MANAGED_HUB in "${MANAGED_HUBS[@]}"; do
  # NOTE: The YAML file is added in-line below to simplify deployment with a single script file.
  #       Any changes below must be synchronized with the setup.sh script.
  # oc delete -n ${MANAGED_HUB} -f ./federation-managed-hub-config.yaml
  oc delete -n "${MANAGED_HUB}" -f - << EOF
apiVersion: work.open-cluster-management.io/v1
kind: ManifestWork
metadata:
  labels:
    app: ocm-search
  name: search-global-config
spec:
  workload:
    manifests:
    - apiVersion: rbac.authorization.k8s.io/v1
      kind: ClusterRoleBinding
      metadata:
        labels:
          app: ocm-search
        name: search-global-binding
      roleRef:
        apiGroup: rbac.authorization.k8s.io
        kind: ClusterRole
        name: global-search-user
      subjects:
      - kind: ServiceAccount
        name: search-global
        namespace: open-cluster-management-agent-addon
    - apiVersion: route.openshift.io/v1
      kind: Route
      metadata:
        labels:
          app: ocm-search
        name: search-global-hub
        namespace: open-cluster-management
      spec:
        port:
          targetPort: search-api
        tls:
          termination: passthrough
        to:
          kind: Service
          name: search-search-api
          weight: 100
---
apiVersion: addon.open-cluster-management.io/v1alpha1
kind: ManagedClusterAddOn
metadata:
  name: managed-serviceaccount
  labels:
    app: ocm-search
spec:
  installNamespace: open-cluster-management-agent-addon
---
apiVersion: authentication.open-cluster-management.io/v1beta1
kind: ManagedServiceAccount
metadata:
  name: search-global
  labels:
    app: ocm-search
spec:
  rotation: {}
EOF
done 

# Disable global search feature in the console.
oc patch configmap console-mce-config -n multicluster-engine -p '{"data": {"globalSearchFeatureFlag": "disabled"}}'
oc patch configmap console-config -n open-cluster-management -p '{"data": {"globalSearchFeatureFlag": "disabled"}}'

# Disable federated search feature in the search-api.
oc patch search search-v2-operator -n open-cluster-management --type='merge' -p '{"spec":{"deployments":{"queryapi":{"envVar":[{"name":"FEATURE_FEDERATED_SEARCH", "value":"false"}]}}}}'

# Disable the ManagedServiceAccount feature in multicluster-engine.
# First check if there are any instances of the ManagedServiceAccount resource.
if oc get managedserviceaccount --all-namespaces --no-headers; then
  echo ""
  echo "Found instances of ManagedServiceAccount resource. This script won't disable the Managed Service Account feature because it appears to be in use."
  echo "Please review the use of ManagedServiceAccount and, if needed, disable it manually using the following patch command:"
  echo ""
  echo "oc patch multiclusterengine multiclusterengine --type='json' -p='[{\"op\": \"add\", \"path\": \"/spec/overrides/components\", \"value\": [{\"name\": \"managedserviceaccount\", \"enabled\": false}]}]'"
else
  oc patch multiclusterengine multiclusterengine --type='json' -p='[{"op": "add", "path": "/spec/overrides/components", "value": [{"name": "managedserviceaccount", "enabled": false}]}]'
  echo "Disabled the Managed Service Account add-on in MulticlusterEngine."
  echo "✅ Global Search feature has been disabled and resources have been deleted." 
fi

