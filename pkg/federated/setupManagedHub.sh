#!/bin/bash
# Copyright Contributors to the Open Cluster Management project

echo "!!! DEPRECATED: Use setup.sh instead."

# # Display target cluster info and ask for confirmation to continue.
# echo ""
# echo "This script will configure Global Search access to the Red Hat Advanced Cluster Management hub at:"
# echo ""
# oc cluster-info | grep "Kubernetes"
# echo ""
# read -p "Are you sure you want to continue? (y/n) " -r $REPLY
# echo ""
# if [[ ! $REPLY =~ ^[Yy]$ ]]; then
#   echo "Setup cancelled. Exiting."
#   exit 1
# fi

# # Validate that ACM 2.9.0 or later is installed.
# echo "Validating Red Hat Advanced Cluster Management version 2.9 or later..."
# ACM_VERSION=$(oc get mch -A -o custom-columns=VERSION:.status.currentVersion --no-headers)
# if ! printf '2.9.0\n%s\n' $ACM_VERSION | sort -V -C; then 
#   echo "Red Hat Advanced Cluster Management 2.9.0 or later is required. Found version $ACM_VERSION. Please upgrade your RHACM installation."
#   exit 1
# fi

# echo "Creating service account and route to access data from Global Search..."
# OCM_NAMESPACE=$(oc get mch -A -o custom-columns=NAMESPACE:.metadata.namespace --no-headers)
# echo "MulticlusterHub found at namespace: $OCM_NAMESPACE"
# echo ""
# oc create route passthrough search-api --service=search-search-api -n $OCM_NAMESPACE
# oc create serviceaccount search-global -n $OCM_NAMESPACE
# oc create clusterrolebinding search-global --clusterrole=global-search-user --serviceaccount=open-cluster-management:search-global

# # Gather the configuration to use in Global Search cluster.
# ROUTE=$(oc get route search-api -n $OCM_NAMESPACE -o jsonpath='{.spec.host}')
# # TODO: Discuss token duration and refresh strategy
# TOKEN=$(oc create token search-global -n $OCM_NAMESPACE --duration=168h)
# TLS_CERT=$(oc get secret search-api-certs -n $OCM_NAMESPACE -oyaml |yq '.data."tls.crt"')
# TLS_KEY=$(oc get secret search-api-certs -n $OCM_NAMESPACE -oyaml |yq '.data."tls.key"')

# echo ""
# echo "[MANUAL STEP] For local development, set the following environment variable:"
# echo ""
# echo "export NAME1="
# echo "export API1=https://$ROUTE/searchapi/graphql"
# echo "export TOKEN1=$TOKEN"
# echo "export TLS_CRT1=$TLS_CERT"
# echo "export TLS_KEY1=$TLS_KEY"
# echo ""
# # TODO: Store credentials in a secret on the Global Search cluster
# echo "[NOT IMPLEMENTED] Add the following configuration to the secret search-federated on the Global Search cluster."
# echo ""
# echo "  - name: <managed cluster name>"
# echo "    url: $ROUTE"
# echo "    token: $TOKEN"
# echo "    tlsCert: $TLS_CERT"
# echo "    tlsKey: $TLS_KEY"
# echo ""
