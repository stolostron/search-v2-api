#!/bin/bash
# Copyright Contributors to the Open Cluster Management project

# Install the Managed Service Account addon using Helm
helm repo add ocm https://openclustermanagement.blob.core.windows.net/releases/
helm repo update
helm search repo ocm/managed-serviceaccount
helm install -n open-cluster-management-addon --create-namespace managed-serviceaccount ocm/managed-serviceaccount

# Get a list of all managed hubs.
# TODO: How do I filter out managed clusters that are not hubs?
MANAGED_HUBS=$(oc get managedcluster -o custom-columns=:.metadata.name --no-headers |grep -v local-cluster)
# Configure each Managed Hub.
for MANAGED_HUB in "${MANAGED_HUBS[@]}"; do
  # Create config resources for each managed hub.
  oc apply -n ${MANAGED_HUB} -f ./search-federation-config.yaml
done

# Clean up
# for MANAGED_HUB in "${MANAGED_HUBS[@]}"; do
#   # Delete the config resources for each managed hub.
#   oc delete -n ${MANAGED_HUB} -f ./search-federation-config.yaml
# done 