# Copyright Contributors to the Open Cluster Management project


default::
	make help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-25s\033[0m %s\n", $$1, $$2}'

setup: ## Generate ssl certificate for development.
	cd sslcert; openssl req -x509 -nodes -days 365 -newkey rsa:2048 -keyout tls.key -out tls.crt -config req.conf -extensions 'v3_req'

gqlgen: ## Generate graphql model. See: https://gqlgen.com/
	go run github.com/99designs/gqlgen generate

.PHONY: run
run: ## Run the service locally.
	PLAYGROUND_MODE=true go run main.go --v=5

.PHONY: lint
lint: ## Run lint and gosec tools.
	go get github.com/golangci/golangci-lint/cmd/golangci-lint@v1.38.0
	golangci-lint run
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

N_USERS ?=2
HOST ?= $(shell oc get route search-api -o custom-columns=host:.spec.host --no-headers -n open-cluster-management --ignore-not-found=true)
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

check-locust: ## Checks if Locust is installed in the system.
ifeq (,$(shell which locust))
	@echo The scale tests require Locust.io, but locust was not found.
	@echo Install locust to continue. For more info visit: https://docs.locust.io/en/stable/installation.html
	exit 1
endif
