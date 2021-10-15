# Inspiration:
# - https://devhints.io/makefile
# - https://tech.davis-hansson.com/p/make/
# - https://marmelab.com/blog/2016/02/29/auto-documented-makefile.html

# Default - top level rule is what gets run when you run just 'make' without specifying a goal/target.
.DEFAULT_GOAL := build

# Make will delete the target of a rule if it has changed and its recipe exits with a nonzero exit status, just as it
# does when it receives a signal.
.DELETE_ON_ERROR:

# When a target is built, all lines of the recipe will be given to a single invocation of the shell rather than each
# line being invoked separately.
.ONESHELL:

# If this variable is not set, the program '/bin/sh' is used as the shell.
SHELL := bash

# The default value of .SHELLFLAGS is -c normally, or -ec in POSIX-conforming mode.
# Extra options are set for Bash:
#   -e             Exit immediately if a command exits with a non-zero status.
#   -u             Treat unset variables as an error when substituting.
#   -o pipefail    The return value of a pipeline is the status of the last command to exit with a non-zero status,
#                  or zero if no command exited with a non-zero status.
.SHELLFLAGS := -euo pipefail -c

# Eliminate use of Make's built-in implicit rules.
MAKEFLAGS += --no-builtin-rules

# Issue a warning message whenever Make sees a reference to an undefined variable.
MAKEFLAGS += --warn-undefined-variables

# Check that the version of Make running this file supports the .RECIPEPREFIX special variable.
# We set it to '>' to clarify inlined scripts and disambiguate whitespace prefixes.
# All script lines start with "> " which is the angle bracket and one space, with no tabs.
ifeq ($(origin .RECIPEPREFIX), undefined)
  $(error This Make does not support .RECIPEPREFIX. Please use GNU Make 4.0 or later.)
endif

.RECIPEPREFIX = >

# Configure an 'all' target to cover the bases.
all: test lint build ## Test and lint and build.
.PHONY: all

# Bring in variables from .env file, ignoring errors if it does not exist
-include .env

# GNU make knows how to execute several recipes at once.
# Normally, make will execute only one recipe at a time, waiting for it to finish before executing the next.
# However, the '-j' or '--jobs' option tells make to execute many recipes simultaneously.
# With no argument, make runs as many recipes simultaneously as possible.
MAKEFLAGS += --jobs

binary_name := smg
gcp_project := $(CLOUDSDK_CORE_PROJECT)
gcp_region := $(CLOUDSDK_RUN_REGION)
image_repository := europe-docker.pkg.dev/$(gcp_project)/smg-eu
image_name := $(shell basename $(CURDIR))
image := $(image_repository)/$(image_name)

# Adjust the width of the first column by changing the '-20s' value in the printf pattern.
help:
> @grep -E '^[a-zA-Z0-9_-]+:.*? ## .*$$' $(filter-out .env, $(MAKEFILE_LIST)) | sort \
  | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
.PHONY: help

# Tests look for sentinel files to determine whether or not they need to be run again.
# If any Go code file has been changed since the sentinel file was last touched, it will trigger a retest.
test: tmp/.tests-passed.sentinel ## Run tests.
test-cover: tmp/.cover-tests-passed.sentinel ## Run all tests with the race detector and output a coverage profile.
bench: tmp/.benchmarks-ran.sentinel ## Run enough iterations of each benchmark to take ten seconds each.

# Linter checks look for sentinel files to determine whether or not they need to check again.
# If any Go code file has been changed since the sentinel file was last touched, it will trigger a rerun.
lint: tmp/.linted.sentinel ## Lint the Dockerfile and all of the Go code. Will also test.

# Builds look for image ID files to determine whether or not they need to build again.
# If any Go code file has been changed since the image ID file was last touched, it will trigger a rebuild.
build: out/image-id ## [DEFAULT] Build the Docker image. Will also test and lint.

build-binary: $(binary_name) ## Build a bare binary only, without a Docker image wrapped around it.

push: tmp/.image-pushed.sentinel ## Push the built image to Artifact Registry on Google Cloud.
deploy: tmp/.cloud-deployed.sentinel ## Deploy the image in Artifact Registry to Cloud Run.

.PHONY: all test test-cover bench lint build build-binary push deploy

clean: ## Clean up the built binary, test coverage, and the temp and output sub-directories.
> go clean -x -v
> rm -rf cover.out tmp out
.PHONY: clean

clean-docker: ## Clean up any local built Docker images and the volume used for caching golangci-lint.
> docker images \
  --filter=reference=$(image) \
  --no-trunc --quiet | sort --ignore-case --unique | xargs -n 1 docker rmi --force
> docker volume rm golangci-lint-cache || true
> rm -f out/image-id
.PHONY: clean-docker

clean-gcr: ## Clean up any remotely built images.
> gcloud container images list-tags $(image_repository) --filter "NOT tags:*" --format="get(digest)" \
  | awk '{ print "$(image_repository)@"$$1 }' \
  | xargs -n 100 gcloud container images delete --quiet
> gcloud artifacts docker images list $(image_repository) \
  --filter "NOT tags:*" --format="value(version)" --include-tags \
  | awk '{ print "$(image)@"$$1 }' \
  | xargs -n 1 gcloud artifacts docker images delete --async --quiet
.PHONY: clean-gcr

clean-all: clean clean-docker clean-gcr ## Clean all of the things.
.PHONY: clean-all

# Tests - re-run if any Go files have changes since 'tmp/.tests-passed.sentinel' was last touched.
tmp/.tests-passed.sentinel: $(shell find . -type f -iname "*.go") go.mod go.sum
> mkdir -p $(@D)
> go test -v ./...
> touch $@

tmp/.cover-tests-passed.sentinel: $(shell find . -type f -iname "*.go") go.mod go.sum
> mkdir -p $(@D)
> go test -count=1 -covermode=atomic -coverprofile=cover.out -race -v ./...
> touch $@

tmp/.benchmarks-ran.sentinel: $(shell find . -type f -iname "*.go") go.mod go.sum
> mkdir -p $(@D)
> go test -bench=. -benchmem -benchtime=10s -run=DoNotRunTests -v ./...
> touch $@

# Lint - re-run if the tests have been re-run (and so, by proxy, whenever the source files have changed).
# These checks are all read-only and will not make any changes.
tmp/.linted.sentinel: tmp/.linted.docker.sentinel tmp/.linted.gofmt.sentinel tmp/.linted.go.vet.sentinel \
  tmp/.linted.golangci-lint.sentinel
> mkdir -p $(@D)
> touch $@

tmp/.linted.docker.sentinel: Dockerfile .hadolint.yaml
> mkdir -p $(@D)
> docker run --env=XDG_CONFIG_HOME=/etc --interactive --pull=always --rm \
  --volume="$(shell pwd)/.hadolint.yaml:/etc/hadolint.yaml:ro" hadolint/hadolint hadolint --verbose - < Dockerfile
> touch $@

tmp/.linted.gofmt.sentinel: tmp/.tests-passed.sentinel
> mkdir -p $(@D)
> find . -type f -iname "*.go" -exec gofmt -d -e -l -s "{}" + \
  | awk '{ print } END { if (NR != 0) { print "Please run \"make gofmt\" to fix these issues!"; exit 1 } }'
> touch $@

tmp/.linted.go.vet.sentinel: tmp/.tests-passed.sentinel
> mkdir -p $(@D)
> go vet ./...
> touch $@

tmp/.linted.golangci-lint.sentinel: .golangci.yaml tmp/.tests-passed.sentinel
> mkdir -p $(@D)
> docker run --env=XDG_CACHE_HOME=/go/cache --interactive --pull=always --rm --volume="$(shell pwd):/app:ro" \
  --volume=golangci-lint-cache:/go --workdir=/app golangci/golangci-lint golangci-lint run --verbose
> touch $@

gofmt: ## Runs 'gofmt -s' to format and simplify all Go code.
> find . -type f -iname "*.go" -exec gofmt -s -w "{}" +
.PHONY: gofmt

# Docker image - re-build if the lint output is re-run (and so, by proxy, whenever the source files have changed).
out/image-id: tmp/.linted.sentinel
> mkdir -p $(@D)
> image_id="$(image):$(shell uuidgen)"
> DOCKER_BUILDKIT=1 docker build --tag="$${image_id}" .
> echo "$${image_id}" > out/image-id

$(binary_name): tmp/.linted.sentinel
> go build -ldflags="-buildid= -w" -trimpath -v -o $(binary_name)

# Auth instructions: https://hub.docker.com/r/google/cloud-sdk/
run-local: out/image-id .env ## Run up the local image.
> docker run --interactive --publish 8080:8080 --rm --tty --volume "$(shell pwd)/.env:/.env:ro" $$(< out/image-id)
.PHONY: run-local

tmp/.image-pushed.sentinel: out/image-id
> mkdir -p $(@D)
> docker tag $$(< out/image-id) $(image):latest
> docker push $(image):latest
> touch $@

tmp/.cloud-deployed.sentinel: tmp/.image-pushed.sentinel .gcloudignore tmp/flags.yaml
> mkdir -p $(@D)
> gcloud run deploy $(binary_name) \
  --allow-unauthenticated \
  --flags-file=tmp/flags.yaml \
  --image="$(image)" \
  --project="$(gcp_project)" \
  --region="$(gcp_region)" \
  --service-account="$(CLOUD_RUN_SERVICE_ACCOUNT)"
> touch $@

tmp/flags.yaml: hack/flags-file.sh .env
> mkdir -p $(@D)
> hack/flags-file.sh
