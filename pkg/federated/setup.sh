#!/bin/bash
# Copyright Contributors to the Open Cluster Management project

# Run this script on the Global Hub cluster to configure Global Search access to the Managed Hub clusters.

# TODO: In the future this should be further automated by either Search or Global Hub operator.

# Install the Managed Service Account addon using Helm.
oc get managedserviceaccount > /dev/null 2>&1
if [ $? -eq 0 ]; then
  echo "Managed Service Account addon already installed."
else
  echo "Installing Managed Service Account addon..."
  helm repo add ocm https://openclustermanagement.blob.core.windows.net/releases/
  helm repo update
  helm search repo ocm/managed-serviceaccount
  helm install -n open-cluster-management-addon --create-namespace managed-serviceaccount ocm/managed-serviceaccount
fi

# Create local config resources for Global Search.
echo "Creating local config resources for Global Search..."
oc apply -f ./federation-config.yaml

# Create config resources for each Managed Hub.
echo "Creating config resources for each Managed Hub..."
MANAGED_HUBS=($(oc get managedcluster -o json | jq -r '.items[] | select(.status.clusterClaims[] | .name == "hub.open-cluster-management.io" and .value != "NotInstalled") | .metadata.name'))
for MANAGED_HUB in "${MANAGED_HUBS[@]}"; do
  echo "Configuring managed hub: ${MANAGED_HUB}"
  oc apply -f ./federation-managed-hub-config.yaml -n "${MANAGED_HUB}"
done
