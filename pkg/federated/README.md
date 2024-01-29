# Federated Search

Use federated search to query and combine results from multiple Red Hat Advanced Cluster Management.

## Pre-requisites  
1. Multicluster Global Hub Operator 1.0 or later.  
2. Red Hat Advanced Cluster Management 2.10 or later.  

## Setup
Execute the script at `./setup.sh` to configure Global Search on the Global Hub cluster.  

The script automates the following steps:
  1. Enable the Managed Service Account add-on in the MulticlusterEngine CR.
  2. Create a service account and secret to access resources managed from the Global Hub cluster.
  3. Create a route and managed service acount on each managed hub to access resources managed by each managed hub.
  4. Configure the Console to use the Global Search API.

> NOTE:    
> You must re-run this script when a Managed Hub is added.    
> This setup is required for Development Preview, it will be fully automated for GA.