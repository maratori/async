help: ## show this message
	@echo "All commands can be run on local machine as well as inside dev container."
	@echo ""
	@sed -nE 's/^ *([^[:blank:]]+)[[:blank:]]*:[^#]*##[[:blank:]]*(.+)/\1\n\2/p' $(MAKEFILE_LIST) | tr '\n' '\0' | xargs -0 -n 2 printf '%-25s%s\n'
.PHONY: help

.DEFAULT_GOAL := help

test: ## run all tests
	@echo "+ $@"
	go test -race -count 1 -p 8 -parallel 8 -timeout 1m ./...
.PHONY: test

test-cover: ## run all tests with code coverage
	@echo "+ $@"
	go test -race -count 1 -p 8 -parallel 8 -timeout 1m -coverpkg ./... -coverprofile coverage.out ./...
.PHONY: test-cover

test-latest-deps: ## run all tests with latest dependencies
	@echo "+ $@"
	go test -modfile .github/latest-deps/go.mod -race -p 8 -parallel 8 -timeout 1m ./...
.PHONY: test

lint: build-docker-dev ## run linter
	@echo "+ $@"
	$(RUN_IN_DOCKER) golangci-lint run
.PHONY: lint

bash: build-docker-dev ## run bash inside container for development
 ifndef INSIDE_DEV_CONTAINER
	@echo "+ $@"
	$(RUN_IN_DOCKER) bash
 endif
.PHONY: bash

IMPORTS = find . -name '*.go' -not -path './.github/*' -exec sed -e '/^import (/,/^)/!d' {} + | sed -e '/\./!d' | grep -v "`head -n 1 go.mod | sed -e 's/module //'`" | sed -E -e 's/\t(.+ )?"/\t_ "/' | sort | uniq | xargs -0 printf 'package imports\n\nimport (\n%s)\n'

tidy: ## keep go.mod and .github/latest-deps tidy
	@echo "+ $@"
	go mod tidy
	$(IMPORTS) > .github/latest-deps/imports.go
	go mod tidy -modfile=.github/latest-deps/go.mod
.PHONY: check

check-tidy: ## ensure go.mod is tidy
	@echo "+ $@"
	cp go.mod go.check.mod
	cp go.sum go.check.sum
	go mod tidy -modfile=go.check.mod
	diff -u go.mod go.check.mod
	diff -u go.sum go.check.sum
	rm go.check.mod go.check.sum

	$(IMPORTS) > .github/latest-deps/imports.check.go
	diff -u .github/latest-deps/imports.go .github/latest-deps/imports.check.go
	rm .github/latest-deps/imports.check.go

	cp .github/latest-deps/go.mod go.check.mod
	cp .github/latest-deps/go.sum go.check.sum
	go mod tidy -modfile=go.check.mod
	diff -u .github/latest-deps/go.mod go.check.mod
	diff -u .github/latest-deps/go.sum go.check.sum
	rm go.check.mod go.check.sum
.PHONY: check-tidy

build-docker-dev: ## build development image from Dockerfile.dev
 ifndef INSIDE_DEV_CONTAINER
	@echo "+ $@"
	DOCKER_BUILDKIT=1 docker build --tag async:dev - < Dockerfile.dev
 endif
.PHONY: build-docker-dev

ifndef INSIDE_DEV_CONTAINER
  RUN_IN_DOCKER = docker run --rm                                                                \
                             -it                                                                 \
                             -w /app                                                             \
                             --mount type=bind,consistency=delegated,source="`pwd`",target=/app  \
                             async:dev
endif
