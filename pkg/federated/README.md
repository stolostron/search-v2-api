# Federated Search

Use federated search to query and combine results from multiple Red Hat Advanced Cluster Management.

**Feature Status (2/20/2024): Development Preview**

## Pre-requisites  
1. Multicluster Global Hub Operator 1.0.0 or later.  
2. Red Hat Advanced Cluster Management 2.10.0 or later.  
    - Managed Hub clusters must have RHACM 2.9.0 or later.

## Setup
Execute the script at `./setup.sh` to configure Global Search on the Global Hub cluster. 

```bash
bash <(curl -s https://raw.githubusercontent.com/stolostron/search-v2-api/main/pkg/federated/setup.sh) 
``` 

The setup script automates the following steps:
  1. Enable the Managed Service Account add-on in the MulticlusterEngine CR.
  2. Create a managed service account for each Managed Hub.
  3. Create role bindings for the managed service account on each Managed Hub.
  4. Create a route on each Managed Hub for the Global Hub to acess the search API.
  5. Configure the Console to use the Global Search API.

> NOTES:  
> Must run setup script using an account with role `open-cluster-management:admin-aggregate` or higher.  
> Must re-run setup script when a Managed Hub is added.  
> The setup script is used only for Development Preview, it will be fully automated for GA.  

## Uninstall
Execute the script at `./cleanup.sh` to remove the Global Search configuration from the Global Hub and Managed Hub clusters. 

```bash
bash <(curl -s https://raw.githubusercontent.com/stolostron/search-v2-api/main/pkg/federated/cleanup.sh) 
``` 

## Limitations

Known limitations for Development Preview.

1. When a Managed Hub is added, resources won't appear on global search. Re-run ./setup.sh to update configuration.
2. When a Managed Hub is offline, the console will show an error. Re-run ./setup.sh to resolve.
3. Filter by Managed Hub is not implemented.
4. LIMIT and SORT for aggregated results is not implemented.
5. Resource YAML and log views are not implemented in the console. This is removed when Global Search is enabled.
6. Actions on resources (delete, edit) are not implemented in the console. This is removed when Global Search is enabled.
