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
#HOST ?=localhost:4010
HOST ?=search-api-open-cluster-management.apps.sno-410-2xlarge-7s7tl.dev07.red-chesterfield.com 
test-scale: check-locust ## Sends multiple simulated requests for testing using Locust. Use N_USERS to change the number of simulated users.
	cd test; locust --headless --users ${N_USERS} --spawn-rate ${N_USERS} -H https://${HOST} -f locust-users.py

test-scale-ui: check-locust ## Start Locust and opens the UI to drive scale tests.
	open http://0.0.0.0:8089/
	cd test; locust --users ${N_USERS} --spawn-rate ${N_USERS} -H https://${HOST} -f locust-users.py

check-locust: ## Checks if Locust is installed in the system.
ifeq (,$(shell which locust))
	@echo The scale tests require Locust.io, but locust was not found.
	@echo Install locust to continue. For more info visit: https://docs.locust.io/en/stable/installation.html
	exit 1
endif
