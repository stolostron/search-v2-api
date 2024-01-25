#!/bin/bash
# Copyright Contributors to the Open Cluster Management project

# Run this script on the Global Hub cluster to configure Global Search access to the Managed Hub clusters.

# TODO: In the future this should be further automated by either Search or Global Hub operator.

echo "Configuring Global Search on the following cluster."
oc cluster-info | grep "Kubernetes"
echo ""
echo "This script will execute the following actions:"
echo "  1. Enable federated search feature in console and search-api."
echo "  2. Enable the Managed Service Account add-on in the MulticlusterEngine CR."
echo "  3. Create a route and managed service acount on each managed hub to access resources managed by each managed hub."
echo ""
read -p "Do you want to continue? (y/n) " -r $REPLY
echo ""
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
  echo "Setup cancelled. Exiting."
  exit 1
fi

# Validate that MulticlusterGlobalHub is installed.
oc get multiclusterglobalhub > /dev/null 2>&1
if [ $? -ne 0 ]; then
  echo "ERROR: MulticlusterGlobalHub is not installed in the target cluster. Exiting."
  exit 1
fi

echo "Enabling global search feature in the console and search-api..."
# Enable global search feature in the console.
oc patch configmap console-mce-config -n multicluster-engine -p '{"data": {"globalSearchFeatureFlag": "enabled"}}'
# Enable federated search feature in the search-api.
oc patch search search-v2-operator -n open-cluster-management --type='merge' -p '{"spec":{"deployments":{"queryapi":{"envVar":[{"name":"FEATURE_FEDERATED_SEARCH", "value":"true"}]}}}}'


# Enable the Managed Service Account add-on in the MultiClusterEngine CR.
oc get managedserviceaccount > /dev/null 2>&1
if [ $? -eq 0 ]; then
  echo "Managed Service Account add-on is enabled."
else
  oc patch multiclusterengine multiclusterengine --type='json' -p='[{"op": "add", "path": "/spec/overrides/components", "value": [{"name": "managedserviceaccount", "enabled": true}]}]'
  echo "Enabled Managed Service Account add-on in MulticlusterEngine. Waiting up to 60 seconds for the addon to be installed..."
  
  # Wait for the Managed Service Account addon to be installed.
  i=0
  while [ $i -lt 60 ]; do
    echo -n "."
    sleep 1
    i=$((i+1))
    oc get managedserviceaccount > /dev/null 2>&1
    if [ $? -eq 0 ]; then
      echo "Managed Service Account add-on is enabled."
      break
    fi
  done
fi

# Create config resources for each Managed Hub.
echo "Creating config resources for each Managed Hub..."
MANAGED_HUBS=($(oc get managedcluster -o json | jq -r '.items[] | select(.status.clusterClaims[] | .name == "hub.open-cluster-management.io" and .value != "NotInstalled") | .metadata.name'))
if [ ${#MANAGED_HUBS[@]} -eq 0 ]; then
  echo "No managed hubs found. Exiting."
  exit 1
fi
for MANAGED_HUB in "${MANAGED_HUBS[@]}"; do
  # FUTURE: Validate that the Managed Hub is running ACM 2.9.0 or later.
  # ACM_VERSION=$(oc get mch -A -o custom-columns=VERSION:.status.currentVersion --no-headers)
  echo "Configuring managed hub: ${MANAGED_HUB}"
  oc apply -f ./federation-managed-hub-config.yaml -n "${MANAGED_HUB}"
done
