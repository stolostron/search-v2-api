#!/bin/bash
# Copyright Contributors to the Open Cluster Management project

# Run this script on the Global Hub cluster to configure Global Search access to the Managed Hub clusters.

# TODO: In the future this should be further automated by either Search or Global Hub operator.

echo "Configuring Global Search access to the Managed Hub clusters."
oc cluster-info | grep "Kubernetes"
read -p "Are you sure you want to continue? (y/n) " -r $REPLY
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


# Enable the Managed Service Account addon in the MultiClusterEngine CR.
oc get managedserviceaccount > /dev/null 2>&1
if [ $? -eq 0 ]; then
  echo "Managed Service Account add-on is enabled."
else
  oc patch multiclusterengine multiclusterengine --type='json' -p='[{"op": "add", "path": "/spec/overrides/components", "value": [{"name": "managedserviceaccount", "enabled": true}]}]'
  echo "Enabled Managed Service Account add-on in MulticlusterEngine. Waiting for the addon to be installed..."
  
  # Wait for the Managed Service Account addon to be installed.
  i=0
  while [ $i -lt 30 ]; do
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

# Create local config resources for Global Search.
echo "Creating local config resources for Global Search..."
oc apply -f ./federation-config.yaml

# Create config resources for each Managed Hub.
echo "Creating config resources for each Managed Hub..."
MANAGED_HUBS=($(oc get managedcluster -o json | jq -r '.items[] | select(.status.clusterClaims[] | .name == "hub.open-cluster-management.io" and .value != "NotInstalled") | .metadata.name'))
if [ ${#MANAGED_HUBS[@]} -eq 0 ]; then
  echo "No managed hubs found. Exiting."
  exit 1
fi
for MANAGED_HUB in "${MANAGED_HUBS[@]}"; do
  # TODO: Validate that the Managed Hub is running ACM 2.9.0 or later.
  # echo "Validating Red Hat Advanced Cluster Management version 2.9 or later..."
  # ACM_VERSION=$(oc get mch -A -o custom-columns=VERSION:.status.currentVersion --no-headers)
  # if ! printf '2.9.0\n%s\n' $ACM_VERSION | sort -V -C; then 
  #   echo "Red Hat Advanced Cluster Management 2.9.0 or later is required. Found version $ACM_VERSION. Please upgrade your RHACM installation."
  #   exit 1
  # fi
  echo "Configuring managed hub: ${MANAGED_HUB}"
  oc apply -f ./federation-managed-hub-config.yaml -n "${MANAGED_HUB}"
done
