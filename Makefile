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
	go run main.go -v=9 playground

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

