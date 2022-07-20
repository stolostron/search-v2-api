# search-v2-api
For development
===============
1. Clone the repo and open the folder
`cd search-v2-api`
2. Run make setup to set up certificates
`make setup`
3. Run make run
`make run`
4. Open the graphql link in browser.
5. Issue a search query in the input.
Example:
```
query{
    search(input: [
        {
            filters: [
                {
                    property: "kind",
                    values:[ "pod" ]
                }
            ],
        }
    ]) {
    		related{
          kind
          count
        }
        count 
    }
}
```
And the output appears on the right hand side.


  
Run within cluster
==================

1. Build the search-v2-api code and push to your private repo in quay.

   &nbsp;`make docker-build`

   &nbsp;`docker tag search-v2-api:latest quay.io/<your_id>/search-v2-api:latest`  

   &nbsp;`docker push quay.io/<your_id>/search-v2-api:latest`  

2. Setup Postgres on your cluster using these [instructions](https://access.crunchydata.com/documentation/postgres-operator/v5/quickstart/). Create postgres service by applying the yaml below.
```
apiVersion: v1
kind: Service
metadata:
  labels:
    postgres-operator.crunchydata.com/cluster: hippo
    postgres-operator.crunchydata.com/role: primary
  name: hippo-primary-ocm
  namespace: postgres-operator
spec:
  ports:
  - name: postgres
    port: 5432
    protocol: TCP
    targetPort: postgres
  selector:
    postgres-operator.crunchydata.com/cluster: hippo
    postgres-operator.crunchydata.com/role: master
```

3. Replace the search-v2-api image deployment image with the image you pushed in Step 1. Note that you have to add your quay image pull secret to the deployment so that this private image can be pulled successfully.Update the TLS secret path in volumemount.

4. Add the environment variables required for connection to Postgres database in the search-v2-api deployment.

```
      containers:
      - env:
        - name: DB_PASSWORD
          value: **************
        - name: DB_HOST
          value: hippo-primary-ocm.postgres-operator.svc.cluster.local
        - name: DB_USER
          value: hippo
        - name: DB_NAME
          value: hippo
        - name: DB_PORT
          value: "5432"
        image: quay.io/<your_id>/search-v2-api:latest
        name: search-v2-api
      imagePullSecrets:
      - name: my-quay-image-pull-secret
      - name: multiclusterhub-operator-pull-secret
```

Rebuild Date: 2022-07-13
