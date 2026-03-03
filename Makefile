SHELL := /bin/bash

GO_VERSION=$(shell grep "^go " go.mod | awk '{print $$2}')

define load_env
	@set -a && . $(1) && set +a && \
	$(MAKE) --no-print-directory $(2)
endef

GIT_SHA=$(shell git rev-parse --short HEAD)

LDFLAGS = -s -w -X main.appVersion=dev-$(GIT_SHA)
PROJECT = $(shell basename $(PWD))
BIN = ./bin
SRC = ./cmd
BINARY = $(BIN)/$(PROJECT)
ROOT=$(PWD)

REGISTRY := registry.combobox.cc
REGISTRY_USER ?= combobox
IMAGE_BACKEND := cman
VERSION_FILE := version
NEW_APP_VERSION := $(shell test -f $(VERSION_FILE) && awk -F. '{printf "%d.%d\n",$$1,$$2+1}' $(VERSION_FILE) || echo "0.1")

APP_USER ?= dummy
GOOS ?= linux
GOARCH ?= amd64
GOLANG_VERSION ?= $(GO_VERSION)
GOLANG_IMAGE ?= alpine
TARGET_DISTR_TYPE ?= alpine
TARGET_DISTR_VERSION ?= latest

DOCKER_BUILD_ARGS := \
	--build-arg APP_USER=$(APP_USER) \
	--build-arg GOOS=$(GOOS) \
	--build-arg GOARCH=$(GOARCH) \
	--build-arg GOLANG_VERSION=$(GOLANG_VERSION) \
	--build-arg GOLANG_IMAGE=$(GOLANG_IMAGE) \
	--build-arg TARGET_DISTR_TYPE=$(TARGET_DISTR_TYPE) \
	--build-arg TARGET_DISTR_VERSION=$(TARGET_DISTR_VERSION) \
	--build-arg LDFLAGS='$(LDFLAGS)' \
	--platform $(GOOS)/$(GOARCH)

define USAGE

Usage: make <target>

some of the <targets> are:
  setup                  - Set up the project
  update-deps            - update Go dependencies
  all                    - build + lint + gosec + test
  build                  - build binaries into $(BIN)/
  lint                   - run linters
  gosec                  - Go security checker
  test                   - run tests
  docker-login           - login to $(REGISTRY)
  docker-build           - build docker image
  release                - build and push Docker image for k8s ($(REGISTRY))
  {dev|stage|prod}-up    - run the app in developer | staging | production mode
  down                   - stop the app

endef
export USAGE

define SETUP_HELP

This command will set up Combobox on the local machine. It requires sudo privileges to create required user and group.
See ./scripts/setup-user.sh for details.

What it will do:
    - install dependencies and tools (linter)
    - create a local user for CMan container manager

Press Enter to continue, Ctrl+C to quit
endef
export SETUP_HELP

define CAKE
   \033[1;31m. . .\033[0m
   i i i
  %~%~%~%
  |||||||
-=========-
endef
export CAKE

help:
	@echo "$$USAGE"

setup:
	@echo "$$SETUP_HELP"
	read
	go install github.com/golangci/golangci-lint@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	./scripts/setup-user.sh

update-deps:
	go get -u ./... && go mod tidy

all: build lint test

build:
	mkdir -p $(BIN)
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -trimpath -o $(BIN)/cman $(SRC)

lint:
	golangci-lint run

gosec:
	gosec ./...

test:
	go test ./...	

docker-login:
	@echo "Logging in to $(REGISTRY)"
	docker login --username "$(REGISTRY_USER)" $(REGISTRY)

docker-build:
	echo Go version: $(GO_VERSION)
	docker build --tag $(REGISTRY)/$(IMAGE_BACKEND):$(NEW_APP_VERSION) $(DOCKER_BUILD_ARGS) --target cman-k8s .

release: docker-build
	echo "Pushing image..."
	docker push $(REGISTRY)/$(IMAGE_BACKEND):$(NEW_APP_VERSION)
	printf "\n\nApplication version released: %s\n" "$(NEW_APP_VERSION)" && echo "$(NEW_APP_VERSION)" > version

down:
	-kill $$(cat $(BIN)/cman.pid) && rm $(BIN)/cman.pid

dev-up:
	$(call load_env,.env_dev,run-up)

stage-up:
	$(call load_env,.env_stage,run-up)

prod-up:
	$(call load_env,.env_prod,run-up)

run-up: down
	echo "ROOT=$(ROOT)"
	echo "Building Cman..."
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -trimpath -o $(BIN)/cman $(SRC)
	echo "CMan compiled OK, installing in $(CMAN_BINARY_PATH)"
	CMAN_BIN=$(CMAN_BINARY_PATH) BIN=$(BIN) ROOT=$(ROOT) ./scripts/run-cman.sh

cake:
	printf "%b\n" "$$CAKE"

.PHONY: help setup update-deps all build lint test docker-login docker-build release down dev-up stage-up prod-up run-up cake

$(V).SILENT:
