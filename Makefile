
# Copyright Red Hat
SHELL := /bin/bash

export PROJECT_DIR            = $(shell 'pwd')
export PROJECT_NAME			  = $(shell basename ${PROJECT_DIR})

# Version to apply to generated artifacts (for bundling/publishing). # This value is set by
# GitHub workflows on push to main and tagging and is not expected to be bumped here.
export VERSION ?= 0.0.1

# Image URL to use all building/pushing image targets
IMAGE_TAG_BASE ?= quay.io/stolostron/$(PROJECT_NAME)
export IMG ?= ${IMAGE_TAG_BASE}:${VERSION}

GIT_BRANCH = $(shell git rev-parse --abbrev-ref HEAD)
IMG_COVERAGE ?= ${PROJECT_NAME}-coverage:${GIT_BRANCH}
IMG_E2E_TEST ?= ${PROJECT_NAME}-e2e-test:${GIT_BRANCH}
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:crdVersions=v1"

# Bundle Prereqs
BUNDLE_IMG ?= ${IMAGE_TAG_BASE}-bundle:${VERSION}

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# enable Go modules
export GO111MODULE=on

# Catalog Deploy Namespace
CATALOG_DEPLOY_NAMESPACE ?= cluster-registration-config

# Global things
OS=$(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(shell uname -m | sed 's/x86_64/amd64/g')


# Credentials for Bundle Push
DOCKER_USER ?=
DOCKER_PASS ?=

# For cypress E2E tests
BROWSER ?= chrome


# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go get $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef


.PHONY: controller-gen
## Find or download controller-gen
controller-gen:
ifeq (, $(shell which controller-gen))
	@( \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.8.0 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	)
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif


.PHONY: kustomize
kustomize: ## Download kustomize locally if necessary.
ifeq (, $(shell which kustomize))
	@{ \
	set -e ;\
	KUSTOMIZE_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$KUSTOMIZE_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/kustomize/kustomize/v3@v3.8.7 ;\
	rm -rf $$KUSTOMIZE_GEN_TMP_DIR ;\
	}
KUSTOMIZE=$(GOBIN)/kustomize
else
KUSTOMIZE=$(shell which kustomize)
endif

CURL := $(shell which curl 2> /dev/null)
YQ_VERSION ?= v4.5.1
YQ_URL ?= https://github.com/mikefarah/yq/releases/download/$(YQ_VERSION)/yq_$(OS)_$(ARCH)
YQ ?= ${PWD}/yq
.PHONY: yq/install
## Install yq to ${YQ} (defaults to current directory)
yq/install: %install:
	@[ -x $(YQ) ] || ( \
		echo "Installing YQ $(YQ_VERSION) ($(YQ_PLATFORM)_$(YQ_ARCH)) from $(YQ_URL)" && \
		curl '-#' -fL -o $(YQ) $(YQ_URL) && \
		chmod +x $(YQ) \
		)
	$(YQ) --version


OPERATOR_SDK ?= ${PWD}/operator-sdk
.PHONY: operatorsdk
## Install operator-sdk to ${OPERATOR_SDK} (defaults to the current directory)
operatorsdk:
	@curl '-#' -fL -o ${OPERATOR_SDK} https://github.com/operator-framework/operator-sdk/releases/download/v1.16.0/operator-sdk_${OS}_${ARCH} && \
		chmod +x ${OPERATOR_SDK}



.PHONY: kubebuilder-tools
## Find or download kubebuilder
kubebuilder-tools:
ifeq (, $(shell which kubebuilder))
	@( \
		set -ex ;\
		KUBEBUILDER_TMP_DIR=$$(mktemp -d) ;\
		cd $$KUBEBUILDER_TMP_DIR ;\
		curl -L -o $$KUBEBUILDER_TMP_DIR/kubebuilder https://github.com/kubernetes-sigs/kubebuilder/releases/download/3.1.0/$$(go env GOOS)/$$(go env GOARCH) ;\
		chmod +x $$KUBEBUILDER_TMP_DIR/kubebuilder && mv $$KUBEBUILDER_TMP_DIR/kubebuilder /usr/local/bin/ ;\
	)
endif


OPM = ./bin/opm
.PHONY: opm
## Download opm locally if necessary.
opm:
ifeq (,$(wildcard $(OPM)))
ifeq (,$(shell which opm 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPM)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/v1.19.1/$${OS}-$${ARCH}-opm ;\
	chmod +x $(OPM) ;\
	}
else
OPM = $(shell which opm)
endif
endif


# See https://book.kubebuilder.io/reference/envtest.html.
#    kubebuilder 2.3.x contained kubebuilder and etc in a tgz
#    kubebuilder 3.x only had the kubebuilder, not etcd, so we had to download a different way
# After running this make target, you will need to either:
# - export KUBEBUILDER_ASSETS=$HOME/kubebuilder/bin
# OR
# - sudo mv $HOME/kubebuilder /usr/local
#
# This will allow you to run `make test`
.PHONY: envtest-tools
## Install envtest tools to allow you to run `make test`
envtest-tools:
ifeq (, $(shell which etcd))
		@{ \
			set -ex ;\
			ENVTEST_TMP_DIR=$$(mktemp -d) ;\
			cd $$ENVTEST_TMP_DIR ;\
			K8S_VERSION=1.19.2 ;\
			curl -sSLo envtest-bins.tar.gz https://storage.googleapis.com/kubebuilder-tools/kubebuilder-tools-$$K8S_VERSION-$$(go env GOOS)-$$(go env GOARCH).tar.gz ;\
			tar xf envtest-bins.tar.gz ;\
			mv $$ENVTEST_TMP_DIR/kubebuilder $$HOME ;\
			rm -rf $$ENVTEST_TMP_DIR ;\
		}
endif



#### BUNDLING AND PUBLISHING ####

.PHONY: docker-login
## Log in to the docker registry for ${BUNDLE_IMG}
docker-login:
	@docker login ${BUNDLE_IMG} -u ${DOCKER_USER} -p ${DOCKER_PASS}

#### BUILD, TEST, AND DEPLOY ####

all: manager

check: check-copyright

check-copyright:
	@build/check-copyright.sh

# Build manager binary
manager: fmt vet
	go build -o bin/cluster-registration main.go

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Build the docker image
docker-build: #test
	docker build . -t ${IMG}

# Push the docker image
docker-push:
	docker push ${IMG}

# Tag the IMG as latest and docker push
docker-push-latest:
	docker tag ${IMG} ${IMAGE_TAG_BASE}:latest
	$(MAKE) docker-push IMG=${IMAGE_TAG_BASE}:latest

