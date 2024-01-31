# !/bin/bash
# Copyright Contributors to the Open Cluster Management project

# Delete resources created by setup.sh
MANAGED_HUBS=($(oc get managedcluster -o json | jq -r '.items[] | select(.status.clusterClaims[] | .name == "hub.open-cluster-management.io" and .value != "NotInstalled") | .metadata.name'))

for MANAGED_HUB in "${MANAGED_HUBS[@]}"; do
  oc delete -n ${MANAGED_HUB} -f ./federation-managed-hub-config.yaml
done 

# Disable global search feature in the console.
oc patch configmap console-mce-config -n multicluster-engine -p '{"data": {"globalSearchFeatureFlag": "disabled"}}'
oc patch configmap console-config -n open-cluster-management -p '{"data": {"globalSearchFeatureFlag": "disabled"}}'

# Disable federated search feature in the search-api.
oc patch search search-v2-operator -n open-cluster-management --type='merge' -p '{"spec":{"deployments":{"queryapi":{"envVar":[{"name":"FEATURE_FEDERATED_SEARCH", "value":"false"}]}}}}'
