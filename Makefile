# Inspiration:
# - https://devhints.io/makefile
# - https://tech.davis-hansson.com/p/make/
# - https://marmelab.com/blog/2016/02/29/auto-documented-makefile.html

SHELL := bash

# Default - top level rule is what gets run when you run just 'make' without specifying a goal/target.
.DEFAULT_GOAL := build

.DELETE_ON_ERROR:
.ONESHELL:
.SHELLFLAGS := -euo pipefail -c

MAKEFLAGS += --no-builtin-rules
MAKEFLAGS += --warn-undefined-variables

ifeq ($(origin .RECIPEPREFIX), undefined)
  $(error This Make does not support .RECIPEPREFIX. Please use GNU Make 4.0 or later.)
endif
.RECIPEPREFIX = >

# Bring in variables from .env file, ignoring errors if it does not exist
-include .env

binary_name ?= smg
gcp_project ?= $(CLOUDSDK_CORE_PROJECT)
gcp_region ?= $(CLOUDSDK_RUN_REGION)
image_repository ?= gcr.io/$(gcp_project)/$(shell basename $(CURDIR))

# Adjust the width of the first column by changing the '-20s' value in the printf pattern.
help:
> @grep -E '^[a-zA-Z0-9_-]+:.*? ## .*$$' $(filter-out .env, $(MAKEFILE_LIST)) | sort \
  | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
.PHONY: help

all: test lint build ## Test and lint and build.
test: tmp/.tests-passed.sentinel ## Run tests.
test-cover: tmp/.cover-tests-passed.sentinel ## Run all tests with the race detector and output a coverage profile.
bench: tmp/.benchmarks-ran.sentinel ## Run enough iterations of each benchmark to take ten seconds each.
lint: tmp/.linted.sentinel ## Lint the Dockerfile and all of the Go code. Will also test.
build: out/image-id ## [DEFAULT] Build the Docker image. Will also test and lint.
build-binary: $(binary_name) ## Build a bare binary only, without a Docker image wrapped around it.
build-cloud: tmp/.cloud-built.sentinel ## Build the image with Google Cloud Build.
deploy: tmp/.cloud-deployed.sentinel ## Deploy the image in Google Container Registry to Cloud Run.
.PHONY: all test test-cover bench lint build build-binary build-cloud deploy

clean: ## Clean up the built binary, test coverage, and the temp and output sub-directories.
> go clean -x -v
> rm -rf cover.out tmp out
.PHONY: clean

clean-docker: ## Clean up any locally built images.
> docker images \
  --filter=reference=$(image_repository) \
  --no-trunc --quiet | sort --ignore-case --unique | xargs -n 1 docker rmi --force
> rm -f out/image-id
.PHONY: clean-docker

clean-gcr: ## Clean up any remotely built images.
> gcloud container images list-tags $(image_repository) --filter "NOT tags:*" --format="get(digest)" \
  | awk '{ print "$(image_repository)@"$$1 }' \
  | xargs -n 100 gcloud container images delete --quiet
.PHONY: clean-gcr

clean-hack: ## Clean up binaries under 'hack'.
> rm -rf hack/bin
.PHONY: clean-hack

clean-all: clean clean-docker clean-gcr clean-hack ## Clean all of the things.
.PHONY: clean-all

# Tests - re-run if any Go files have changes since tmp/.tests-passed.sentinel was last touched.
tmp/.tests-passed.sentinel: $(shell find . -type f -iname "*.go") go.mod go.sum
> mkdir -p $(@D)
> go test -v ./...
> touch $@

tmp/.cover-tests-passed.sentinel: $(shell find . -type f -iname "*.go") go.mod go.sum
> mkdir -p $(@D)
> go test -count=1 -covermode=atomic -coverprofile=cover.out -race ./...
> touch $@

tmp/.benchmarks-ran.sentinel: $(shell find . -type f -iname "*.go") go.mod go.sum
> mkdir -p $(@D)
> go test -bench=. -benchmem -benchtime=10s -run=DoNotRunTests ./...
> touch $@

# Lint - re-run if the tests have been re-run (and so, by proxy, whenever the source files have changed).
tmp/.linted.sentinel: Dockerfile .golangci.yaml .hadolint.yaml hack/bin/golangci-lint tmp/.tests-passed.sentinel
> mkdir -p $(@D)
> docker run --env XDG_CONFIG_HOME=/etc --interactive --rm \
  --volume "$(shell pwd)/.hadolint.yaml:/etc/hadolint.yaml:ro" hadolint/hadolint < Dockerfile
> find . -type f -iname "*.go" -exec gofmt -e -l -s "{}" + \
  | awk '{ print } END { if (NR != 0) { print "gofmt found issues in the above file(s); \
please run \"make lint-simplify\" to remedy"; exit 1 } }'
> go vet ./...
> hack/bin/golangci-lint run
> touch $@

lint-simplify: ## Runs 'gofmt -s' to format and simplify all Go code.
> find . -type f -iname "*.go" -exec gofmt -s -w "{}" +
.PHONY: lint-simplify

hack/bin/golangci-lint:
> mkdir -p $(@D)
> curl --fail --location --show-error --silent \
  https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
  | sh -s -- -b $(shell pwd)/hack/bin

# Docker image - re-build if the lint output is re-run.
out/image-id: tmp/.linted.sentinel
> mkdir -p $(@D)
> image_id="$(image_repository):$(shell uuidgen)"
> DOCKER_BUILDKIT=1 docker build --tag="$${image_id}" .
> echo "$${image_id}" > out/image-id

$(binary_name): tmp/.linted.sentinel
> go build -ldflags="-buildid= -w" -trimpath -v -o $(binary_name)

# Auth instructions: https://hub.docker.com/r/google/cloud-sdk/
run-local: out/image-id .env ## Run up the local image.
> docker run --interactive --publish 8080:8080 --rm --tty --volume "$(shell pwd)/.env:/.env:ro" $$(< out/image-id)
.PHONY: run-local

tmp/.cloud-built.sentinel: Dockerfile tmp/.linted.sentinel .gcloudignore cloudbuild.yaml *.gohtml
> mkdir -p $(@D)
> gcloud builds submit \
  --config cloudbuild.yaml \
  --project="$(gcp_project)" \
  --substitutions "_DESTINATION=$(image_repository)"
> touch $@

tmp/.cloud-deployed.sentinel: tmp/.cloud-built.sentinel .gcloudignore tmp/flags.yaml
> mkdir -p $(@D)
> gcloud run deploy $(binary_name) \
  --allow-unauthenticated \
  --flags-file=tmp/flags.yaml \
  --image="$(image_repository)" \
  --project="$(gcp_project)" \
  --region="$(gcp_region)" \
  --service-account="$(CLOUD_RUN_SERVICE_ACCOUNT)"
> touch $@

tmp/flags.yaml: hack/flags-file.sh .env
> mkdir -p $(@D)
> hack/flags-file.sh
