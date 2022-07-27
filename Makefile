help: ## show this message
	@echo "All commands can be run on local machine as well as inside dev container."
	@echo ""
	@echo "`grep -hE '^\S+:.*##' $(MAKEFILE_LIST) | sed -e 's/:.*##\s*/:/' | column -c2 -t -s :`"
.PHONY: help

.DEFAULT_GOAL := help

test: ## run all tests
	@echo "+ $@"
	go test -race -p 8 -parallel 8 -timeout 1m ./...
.PHONY: test

test-cover: ## run all tests with code coverage
	@echo "+ $@"
	go test -race -p 8 -parallel 8 -timeout 1m -coverpkg ./... -coverprofile coverage.out ./...
.PHONY: test-cover

lint: build-docker-dev ## run linter
	@echo "+ $@"
	$(RUN_IN_DOCKER) golangci-lint run
.PHONY: lint

bash: build-docker-dev ## run bash inside container for development
	@echo "+ $@"
	$(RUN_IN_DOCKER) bash
.PHONY: bash

build-docker-dev: ## build development image from Dockerfile.dev
	@echo "+ $@"
	DOCKER_BUILDKIT=1 docker build --tag async:dev - < Dockerfile.dev
.PHONY: build-docker-dev

RUN_IN_DOCKER = docker run --rm                                                                \
                           -it                                                                 \
                           -w /app                                                             \
                           --mount type=bind,consistency=delegated,source="`pwd`",target=/app  \
                           async:dev
