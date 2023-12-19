#!/bin/bash

# This script is used to setup the managed hub cluster for global search.


# Display target cluster info
oc cluster-info

# Ask for confirmation. Press y to continue.
echo ""
echo "This script will configure Global Search access to the Red Hat Advanced Cluster Management hub at:"
oc cluster-info | grep "https"
echo ""
read -p "Are you sure you want to continue? (y/n) " -r $REPLY
echo ""
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
  echo "Exiting."
  exit 1
fi

# Validate that ACM 2.9.0 or later is installed.
ACM_VERSION=$(oc get mch -A -o custom-columns=VERSION:.status.currentVersion --no-headers)
if ! printf '2.9.0\n%s\n' $ACM_VERSION | sort -V -C; then 
  echo "Red Hat Advanced Cluster Management 2.9.0 or later is required. Found version $ACM_VERSION. Please upgrade your RHACM installation."
  exit 1
fi
# TODO: Require TLS validation for this route.
oc create route passthrough search-api --service=search-search-api -n open-cluster-management
oc create serviceaccount global-search -n open-cluster-management
oc create clusterrolebinding global-search --clusterrole=global-search-user --serviceaccount=open-cluster-management:global-search
ROUTE=$(oc get route search-api -n open-cluster-management -o jsonpath='{.spec.host}')
# TODO: Discuss token duration and refresh strategy
TOKEN=$(oc create token global-search -n open-cluster-management --duration=168h)


echo "[MANUAL STEP] For local development, set the following environment variable:"
echo "export TOKEN1=$TOKEN"
echo ""
# TODO: Store credentials in a secret on the Global Search cluster
echo "[NOT IMPLEMENTED] Add the following configuration to the secret search-federated on the Global Search cluster."
echo ""
echo "  - Name: <managed cluster name>"
echo "    Route: $ROUTE"
echo "    Token: $TOKEN"
echo ""
