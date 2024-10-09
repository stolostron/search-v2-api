# Copyright Contributors to the Open Cluster Management project


default::
	make help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-25s\033[0m %s\n", $$1, $$2}'

setup: ## Configure local development environment.
	@echo "Using current OCP target cluster.\\n"
	@echo "$(shell oc cluster-info|grep 'Kubernetes')"
	@echo "\\n1. Generating local self-signed certificate.\\n"
	cd sslcert; openssl req -x509 -nodes -days 365 -newkey rsa:2048 -keyout tls.key -out tls.crt -config req.conf -extensions 'v3_req' &> /dev/null
	@echo "\\n2. [MANUAL STEP] Set these environment variables on your terminal.\\n"
	export DB_NAME=$(shell oc get secret search-postgres -n open-cluster-management -o jsonpath='{.data.database-name}'|base64 -D)
	export DB_USER=$(shell oc get secret search-postgres -n open-cluster-management -o jsonpath='{.data.database-user}'|base64 -D)
	export DB_PASS=$(shell oc get secret search-postgres -n open-cluster-management -o jsonpath='{.data.database-password}'|base64 -D)
	@echo "\\n3. [MANUAL STEP] Start port forwarding.\\n"
	@echo "oc port-forward service/search-postgres -n open-cluster-management 5432:5432 \\n"

gqlgen: ## Generate graphql model. See: https://gqlgen.com/
	go run github.com/99designs/gqlgen generate

.PHONY: run
run: ## Run the service locally.
	PLAYGROUND_MODE=true go run -tags development main.go --v=4

.PHONY: lint
lint: ## Run lint and gosec tools.
	GOPATH=$(go env GOPATH)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b "${GOPATH}/bin" v1.61.0
	CGO_ENABLED=1 GOGC=25 golangci-lint run --timeout=3m
	go mod tidy
	gosec ./...

.PHONY: test
test: ## Run unit tests.
	go test ./... -v -coverprofile cover.out

coverage: test ## Run unit tests and show code coverage.
	go tool cover -html=cover.out -o=cover.html
	open cover.html

docker-build: ## Build the docker image.
	docker build -f Dockerfile . -t search-v2-api

show-metrics:
	curl -k https://localhost:4010/metrics

N_USERS ?=2
HOST ?= $(shell oc get route search-api -o custom-columns=host:.spec.host --no-headers -n open-cluster-management --ignore-not-found=true --request-timeout='1s')
ifeq ($(strip $(HOST)),)
	CONFIGURATION_MSG = @echo \\n\\tThe search-api route was not found in the target cluster.\\n\
	\\tThis test will run against the local instance https://localhost:4010\\n\
	\\tIf you want to run this test against a cluster, create the route with make test-scale-setup\\n;
	
	HOST = localhost:4010
endif
export API_TOKEN :=$(shell oc whoami -t)

test-scale: check-locust ## Simulate multiple users sending requests to the API. Use N_USERS to change the number of simulated users.
	${CONFIGURATION_MSG}
	cd test; locust --headless --users ${N_USERS} --spawn-rate ${N_USERS} -H https://${HOST} -f locust-users.py

test-scale-ui: check-locust ## Start Locust and open the web browser to drive scale tests.
	${CONFIGURATION_MSG}
	open http://0.0.0.0:8090/
	cd test; locust --users ${N_USERS} --spawn-rate ${N_USERS} -H https://${HOST} -f locust-users.py --web-port 8090

test-scale-setup: ## Creates the search-api route in the current target cluster.
	oc create route passthrough search-api --service=search-search-api -n open-cluster-management


# Target API URL for the test queries.
SEARCH_API_URL=https://localhost:4010/searchapi/graphql
FEDERATED ?= ${F}
ifeq (${FEDERATED}, true)
	SEARCH_API_URL=https://localhost:4010/federated
endif

# Specifies the query string to send with the test requests.
# Q is an alias for QUERY.
QUERY ?= ${Q}
QUERY_STR = "{"query":"query Search() { }","variables":{} }"
ifeq (${QUERY}, schema)
	QUERY_STR='{"query":"query Schema() { searchSchema() }","variables":{} }'
else ifeq (${QUERY}, searchComplete)
	QUERY_STR='{"query":"query SearchComplete { searchComplete(property: \"kind\") }","variables":{} }'
else ifeq (${QUERY}, search)
	QUERY_STR='{"query":"query Search($$input: [SearchInput]) { search(input: $$input) { count items } }","variables":{"input":[{"keywords":[],"filters":[{"property":"kind","values":["ConfigMap"]}],"limit": 3}]}}'
else ifeq (${QUERY}, searchAlias)
	QUERY_STR='{"query":"query Search($$input: [SearchInput]) { searchResult: search(input: $$input) { count items __typename } }","variables":{"input":[{"keywords":[],"filters":[{"property":"kind","values":["ConfigMap"]}],"limit": 3}]}}'
else ifeq (${QUERY}, searchCount)
	QUERY_STR='{"query":"query Search($$input: [SearchInput]) { search(input: $$input) { count } }","variables":{"input":[{"keywords":[],"filters":[{"property":"kind","values":["ConfigMap"]}],"limit": 3}]}}'
else ifeq (${QUERY}, searchCompleteAlias)
	QUERY_STR='{"query":"query SearchComplete { aliasedResult: searchComplete(property: \"kind\") }","variables":{} }'
else ifeq (${QUERY}, searchRelated)
	QUERY_STR='{"query":"query Search($$input: [SearchInput]) { search(input: $$input) { count related { kind count items } } }","variables":{"input":[{"keywords":[],"filters":[{"property":"kind","values":["Deployment"]}],"limit": 3}]}}'
else ifeq (${QUERY}, searchRelatedAlias)
	QUERY_STR='{"query":"query Search($$input: [SearchInput]) { searchResult: search(input: $$input) { count related { kind count items } } }","variables":{"input":[{"keywords":[],"filters":[{"property":"kind","values":["Deployment"]}],"limit": 3}]}}'
endif

send: ## Sends a graphQL request using cURL for development and testing. QUERY (alias Q) is a required parameter, values are: [schema|search|searchComplete|searchCount|messages].
ifeq (${QUERY},)
	@echo "QUERY (or Q) is required. Example: make send QUERY=searchComplete"
	@echo "Valid QUERY values: schema, search, searchComplete, searchCount, messages"
	exit 1
endif
	# Sending query with the following parameters:
	# - URL           ${URL}
	# - GRAPHQL QUERY ${QUERY_STR}
	#
	curl --insecure --location --request POST ${SEARCH_API_URL} \
	--header "Authorization: Bearer ${API_TOKEN}" --header 'Content-Type: application/json' \
	--data-raw ${QUERY_STR} | jq

check-locust: ## Checks if Locust is installed in the system.
ifeq (,$(shell which locust))
	@echo The scale tests require Locust.io, but locust was not found.
	@echo Install locust to continue. For more info visit: https://docs.locust.io/en/stable/installation.html
	exit 1
endif
