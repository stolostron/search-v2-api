# !/bin/bash
# Copyright Contributors to the Open Cluster Management project

# Delete resources created by setup.sh

# Delete configuration resources for the Managed Hubs.
oc delete manifestwork -A -l app=ocm-search
oc delete managedclusteraddon -A -l app=ocm-search
oc delete managedserviceaccount -A -l app=ocm-search

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
