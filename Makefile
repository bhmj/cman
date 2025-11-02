SHELL := /bin/bash

define load_env
	@set -a && . $(1) && set +a && \
	$(MAKE) --no-print-directory $(2)
endef

# application binary type for docker image
export DOCKER_GOOS=linux
export DOCKER_GOARCH=amd64
# Go version to use while building binaries for docker image
export GOLANG_VERSION=1.24
# golang OS tag for building binaries for docker image
export GOLANG_IMAGE=alpine 
# target OS: the image type to run in production. Usually alpine fits OK.
export TARGET_DISTR_TYPE=alpine
# target OS version (codename)
export TARGET_DISTR_VERSION=latest
# a user created inside the container
# files created by those services on mounted volumes will be owned by this user
export DOCKER_USER=$(USER)

LDFLAGS = -s -w -X main.appVersion=dev-$(shell git rev-parse --short HEAD)-$(shell date +%y-%m-%d-%H%M%S)
PROJECT = $(shell basename $(PWD))
BIN = ./bin
SRC = ./cmd
BINARY = $(BIN)/$(PROJECT)
ROOT=$(PWD)

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

define DOCKER_PARAMS
--build-arg USER=$(DOCKER_USER) \
--build-arg GOOS=$(DOCKER_GOOS) \
--build-arg GOARCH=$(DOCKER_GOARCH) \
--build-arg GOLANG_VERSION=$(GOLANG_VERSION) \
--build-arg GOLANG_IMAGE=$(GOLANG_IMAGE) \
--build-arg TARGET_DISTR_TYPE=$(TARGET_DISTR_TYPE) \
--build-arg TARGET_DISTR_VERSION=$(TARGET_DISTR_VERSION) \
--build-arg LDFLAGS="$(LDFLAGS)" \
--file Dockerfile
endef
export DOCKER_PARAMS

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

.PHONY: help setup update-deps all build lint test down dev-up stage-up prod-up run-up cake

$(V).SILENT:
