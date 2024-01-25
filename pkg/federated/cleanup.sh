# !/bin/bash
# Copyright Contributors to the Open Cluster Management project

# Delete resources created by setup.sh
MANAGED_HUBS=($(oc get managedcluster -o json | jq -r '.items[] | select(.status.clusterClaims[] | .name == "hub.open-cluster-management.io" and .value != "NotInstalled") | .metadata.name'))

for MANAGED_HUB in "${MANAGED_HUBS[@]}"; do
  oc delete -n ${MANAGED_HUB} -f ./federation-managed-hub-config.yaml
done 

