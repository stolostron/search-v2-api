#!/bin/bash
# Copyright Contributors to the Open Cluster Management project

# Configures Global Search.
# Run this script on an Openshift cluster with the Multicluster Global Hub operator.

# FUTURE: The actions in this script should be moved to either Search or Global Hub operator.


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

echo -e "Validating prerequisites...\n"

# Validate that Red Hat Advanced Cluster Management (MulticlusterHub) 2.10.0 is installed.
REQUIRED_RHACM_VERSION="2.10.0"
oc get multiclusterhub -o yaml > /dev/null 2>&1
if [ $? -ne 0 ]; then
  echo "❌ ERROR: Red Hat Advanced Cluster Management (MulticlusterHub) is not installed in the target cluster, or user doesn't have the required permissions. Exiting."
  exit 1
else
  rhacm_version=$(oc get multiclusterhub -A -o yaml | yq '.items[0].status.currentVersion')
  if ! printf "${REQUIRED_RHACM_VERSION}\n%s\n" $rhacm_version | sort -V -C; then
    echo "❌ ERROR: Red Hat Advanced Cluster Management (MulticlusterHub) ${REQUIRED_RHACM_VERSION} or later is required. Found version ${rhacm_version}. Exiting."
    exit 1
  fi
  echo "✅ Red Hat Advanced Cluster Management (MulticlusterHub) version ${rhacm_version} found."
fi

# Validate that MulticlusterGlobalHub 1.0.1 is installed.
REQUIRED_MCGH_VERSION="1.0.1"
oc get multiclusterglobalhub -o yaml > /dev/null 2>&1
if [ $? -ne 0 ]; then
  echo "❌ ERROR: Multicluster Global Hub is not installed in the target cluster, or user doesn't have the required permissions. Exiting."
  exit 1
else
  mcgh_version=$(oc get multiclusterglobalhub -A -o yaml | yq '.items[0].status.currentVersion')
  if ! printf "${REQUIRED_MCGH_VERSION}\n%s\n" $rhacm_version | sort -V -C; then
    echo "❌ ERROR: Multicluster Global Hub ${REQUIRED_MCGH_VERSION} or later is required. Found version ${rhacm_version}. Exiting."
    exit 1
  fi
  echo "✅ Multicluster Global Hub version ${mcgh_version} found."
fi

echo -e "\nEnabling the global search feature in the console and search-api...\n"
# Enable global search feature in the console.
oc patch configmap console-mce-config -n multicluster-engine -p '{"data": {"globalSearchFeatureFlag": "enabled"}}'
oc patch configmap console-config -n open-cluster-management -p '{"data": {"globalSearchFeatureFlag": "enabled"}}'

# Enable federated search feature in the search-api.
oc patch search search-v2-operator -n open-cluster-management --type='merge' -p '{"spec":{"deployments":{"queryapi":{"envVar":[{"name":"FEATURE_FEDERATED_SEARCH", "value":"true"}]}}}}'


# Enable the Managed Service Account add-on in the MultiClusterEngine CR.
oc get managedserviceaccount > /dev/null 2>&1
if [ $? -eq 0 ]; then
  echo "Managed Service Account add-on is enabled."
else
  oc patch multiclusterengine multiclusterengine --type='json' -p='[{"op": "add", "path": "/spec/overrides/components", "value": [{"name": "managedserviceaccount", "enabled": true}]}]'
  echo "Enabled the Managed Service Account add-on in MulticlusterEngine."
  
  # Wait for the Managed Service Account addon to be installed.
  echo -n "Waiting up to 30 seconds for the add-on to be installed."
  i=0
  while [ $i -lt 30 ]; do
    echo -n "."
    sleep 1
    i=$((i+1))
    oc get managedserviceaccount > /dev/null 2>&1
    if [ $? -eq 0 ]; then
      echo -e "\nManaged Service Account add-on is enabled."
      break
    fi
  done
fi

# Delete any existing configuration for the Managed Hubs. This helps to reset the state if Managed Hubs are removed or offline.
echo -e "\nDeleting existing configuration resources for the Managed Hubs...\n"
oc delete manifestwork -A -l app=ocm-search
oc delete managedclusteraddon -A -l app=ocm-search
oc delete managedserviceaccount -A -l app=ocm-search

# Create configuration resources for each Managed Hub.
echo -e "\nCreating config resources for each Managed Hub...\n"
MANAGED_HUBS=($(oc get managedcluster -o json | jq -r '.items[] | select(.status.clusterClaims[] | .name == "hub.open-cluster-management.io" and .value != "NotInstalled") | .metadata.name'))
if [ ${#MANAGED_HUBS[@]} -eq 0 ]; then
  echo "❌ No managed hubs found. Exiting."
  exit 1
fi
for MANAGED_HUB in "${MANAGED_HUBS[@]}"; do
  # FUTURE: Validate that the Managed Hub is running ACM 2.9.0 or later.
  # ACM_VERSION=$(oc get mch -A -o custom-columns=VERSION:.status.currentVersion --no-headers)
  echo "Configuring managed hub: ${MANAGED_HUB}"
  
  # NOTE: The YAML file was added in-line below to simplify deployment with a single script file.
  #       Any changes below must be synchronized with the setup.sh script.
  # oc apply -f ./federation-managed-hub-config.yaml -n "${MANAGED_HUB}"
  oc apply -n "${MANAGED_HUB}" -f - << EOF
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

  echo ""
done

echo "🚀 Global Search setup complete."
