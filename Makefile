
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
CATALOG_DEPLOY_NAMESPACE ?= kcp-compute-config

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
FQ_APIS=github.com/stolostron/${PROJECT_NAME}/api/singapore/v1alpha1

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

.PHONY: register-gen
## Fdownload register-gen
register-gen:
ifeq (, $(shell which register-gen))
	@( \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get k8s.io/code-generator/cmd/register-gen ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	)
REGISTER_GEN=$(GOBIN)/register-gen
else
REGISTER_GEN=$(shell which register-gen)
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
		curl -L -o $$KUBEBUILDER_TMP_DIR/kubebuilder https://github.com/kubernetes-sigs/kubebuilder/releases/download/v3.1.0/kubebuilder_$$(go env GOOS)_$$(go env GOARCH) ;\
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

ifeq (, $(shell which ginkgo))
	@{ \
	set -ex ;\
	ENVTEST_TMP_DIR=$$(mktemp -d) ;\
	cd $$ENVTEST_TMP_DIR ;\
	go get -u "github.com/onsi/ginkgo/v2/ginkgo@v2.1.1";\
	rm -rf $$ENVTEST_TMP_DIR ;\
	}
endif


#### BUNDLING AND PUBLISHING ####
.PHONY: publish

## Build and push the operator, bundle, and catalog
publish: docker-login docker-build docker-push
	if [[ "${PUSH_LATEST}" = true ]]; then \
		echo "Tagging operator image as latest and pushing"; \
		$(MAKE) docker-push-latest; \
	fi;

.PHONY: docker-login
## Log in to the docker registry for ${BUNDLE_IMG}
docker-login:
	@docker login ${BUNDLE_IMG} -u ${DOCKER_USER} -p ${DOCKER_PASS}

#### BUILD, TEST, AND DEPLOY ####

all: manager

check: check-copyright

check-copyright:
	@build/check-copyright.sh

test: fmt vet manifests envtest-tools
	@ginkgo -r --cover --coverprofile=cover.out --coverpkg ./... &&\
	COVERAGE=`go tool cover -func="cover.out" | grep "total:" | awk '{ print $$3 }' | sed 's/[][()><%]/ /g'` &&\
	echo "-------------------------------------------------------------------------" &&\
	echo "TOTAL COVERAGE IS $$COVERAGE%" &&\
	echo "-------------------------------------------------------------------------" &&\
	go tool cover -html "cover.out" -o ${PROJECT_DIR}/cover.html

# Build manager binary
manager: fmt vet
	go build -o bin/kcp-compute main.go

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Build the docker image
docker-build: manifests #test
	docker build . -t ${IMG}

# Push the docker image
docker-push:
	docker push ${IMG}


# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: kustomize
	cp config/installer/kustomization.yaml config/installer/kustomization.yaml.tmp
	cd config/installer && $(KUSTOMIZE) edit set image controller=${IMG}
	${KUSTOMIZE} build config/default | kubectl apply -f -
	mv config/installer/kustomization.yaml.tmp config/installer/kustomization.yaml

undeploy:
	kubectl delete --wait=true -k config/default

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen yq/install
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..."
	${YQ} e '.metadata.name = "compute-operator-manager-role"' config/rbac/role.yaml > deploy/compute-operator/clusterrole.yaml && \
	${YQ} e '.metadata.name = "leader-election-operator-role" | .metadata.namespace = "{{ .Namespace }}"' config/rbac/leader_election_role.yaml > deploy/compute-operator/leader_election_role.yaml

# Generate code
generate: kubebuilder-tools controller-gen register-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

# Tag the IMG as latest and docker push
docker-push-latest:
	docker tag ${IMG} ${IMAGE_TAG_BASE}:latest
	$(MAKE) docker-push IMG=${IMAGE_TAG_BASE}:latest
