
# Copyright Red Hat
SHELL := /bin/bash

export PROJECT_DIR            = $(shell 'pwd')
export PROJECT_NAME			  = $(shell basename ${PROJECT_DIR})

POD_NAMESPACE ?= compute-config

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

# Skip webhook on kcp until Services are supported
SKIP_WEBHOOK ?= false

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# enable Go modules
export GO111MODULE=on

# Catalog Deploy Namespace
CATALOG_DEPLOY_NAMESPACE ?= compute-config

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
GOBIN=$(PROJECT_DIR)/bin go install $(2) ;\
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
	go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.8.0 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	)
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

.PHONY: register-gen
## Find or download register-gen
register-gen:
ifeq (, $(shell which register-gen))
	@( \
	set -e ;\
	REGISTER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$REGISTER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go install k8s.io/code-generator/cmd/register-gen@v0.23.5 ;\
	rm -rf $$REGISTER_GEN_TMP_DIR ;\
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
	go install sigs.k8s.io/kustomize/kustomize/v3@v3.8.7 ;\
	rm -rf $$KUSTOMIZE_GEN_TMP_DIR ;\
	}
KUSTOMIZE=$(GOBIN)/kustomize
else
KUSTOMIZE=$(shell which kustomize)
endif

.PHONY: applier
## Find or download applier
applier:
ifeq (, $(shell which applier))
	@( \
	set -e ;\
	APPLIER_TMP_DIR=$$(mktemp -d) ;\
	cd $$APPLIER_TMP_DIR ;\
	go mod init tmp ;\
	go install github.com/stolostron/applier@24eb6dde5781bd5521839dd802d3f923bfb2d6fc ;\
	rm -rf $$APPLIER_TMP_DIR ;\
	)
APPLIER=$(GOBIN)/applier
else
APPLIER=$(shell which applier)
endif

CURL := $(shell which curl 2> /dev/null)
YQ_VERSION ?= v4.5.1
YQ_URL ?= https://github.com/mikefarah/yq/releases/download/$(YQ_VERSION)/yq_$(OS)_$(ARCH)
YQ ?= ${PWD}/yq
.PHONY: yq/install
## Install yq to ${YQ} (defaults to current directory)
yq/install: %install:
	@[ -x $(YQ) ] || ( \
	  echo "BUILD_HARNESS_OS $(BUILD_HARNESS_OS) " ; \
	  OC_PLATFORM ?= $(shell echo $(BUILD_HARNESS_OS) | sed 's/darwin/mac/g') ; \
    echo "OC_PLATFORM $(OC_PLATFORM)" ; \
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

.PHONY: kcp-plugin
## Find or download kcp-plugin
## 		git checkout v0.5.0-alpha.1
kcp-plugin:
ifeq (, $(shell kubectl plugin list 2>/dev/null | grep kubectl-kcp))
	@( \
		set -ex ;\
		KCP_TMP_DIR=$$(mktemp -d) ;\
		cd $$KCP_TMP_DIR ;\
		git clone https://github.com/kcp-dev/kcp.git ;\
		cd kcp ;\
		git checkout v0.7.1; \
		make install WHAT="./cmd/kubectl-kcp"; \
		make install WHAT="./cmd/kcp"; \
	)
endif

# Maybe not needed

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
			test -d $$HOME/kubebuilder && rm -rf $$HOME/kubebuilder ;\
			mv $$ENVTEST_TMP_DIR/kubebuilder $$HOME ;\
			rm -rf $$ENVTEST_TMP_DIR ;\
	}
endif

ifeq (, $(shell which ginkgo))
	@{ \
	set -ex ;\
	ENVTEST_TMP_DIR=$$(mktemp -d) ;\
	cd $$ENVTEST_TMP_DIR ;\
	go install "github.com/onsi/ginkgo/v2/ginkgo@v2.1.1";\
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
	@ginkgo -r --cover --coverprofile=coverage.out --coverpkg ./... &&\
	COVERAGE=`go tool cover -func="coverage.out" | grep "total:" | awk '{ print $$3 }' | sed 's/[][()><%]/ /g'` &&\
	echo "-------------------------------------------------------------------------" &&\
	echo "TOTAL COVERAGE IS $$COVERAGE%" &&\
	echo "-------------------------------------------------------------------------" &&\
	go tool cover -html "coverage.out" -o ${PROJECT_DIR}/cover.html

# Build manager binary
manager: fmt vet
	go build -o bin/compute main.go

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet $(go list ./... | grep -v /webhook/)
# go vet ./...

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
	IMAGE=${IMG} SKIP_WEBHOOK=${SKIP_WEBHOOK} ${KUSTOMIZE} build config/default | kubectl apply -f -
	mv config/installer/kustomization.yaml.tmp config/installer/kustomization.yaml

undeploy:
	kubectl delete --wait=true clusterregistrars --all
	kubectl delete --wait=true -k config/default

# Generate manifests e.g. CRD, RBAC etc.

manifests: controller-gen yq/install kcp-plugin generate
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..."
	${YQ} e '.metadata.name = "compute-operator-manager-role"' config/rbac/role.yaml > deploy/compute-operator/clusterrole.yaml && \
	${YQ} e '.metadata.name = "leader-election-operator-role" | .metadata.namespace = "{{ .Namespace }}"' config/rbac/leader_election_role.yaml > deploy/compute-operator/leader_election_role.yaml && \
	kubectl kcp crd snapshot --filename config/crd/singapore.open-cluster-management.io_registeredclusters.yaml --prefix latest \
	> config/apiresourceschema/singapore.open-cluster-management.io_registeredclusters.yaml

samples: applier
# Later we can use `cm apply custom-resources --paths .. --values ... --dry-run --outpute-file ...` to generate the files
	rm -rf hack/compute/*
	cp resources/compute-templates/workspace/* hack/compute
	cp resources/compute-templates/virtual-workspace/* hack/compute
	applier render --paths resources/compute-templates/workspace/apibinding.yaml \
	               --values resources/compute-templates/hack-values.yaml --output-file hack/compute/apibinding.yaml
	applier render --paths resources/compute-templates/virtual-workspace/namespace.yaml \
	               --values resources/compute-templates/hack-values.yaml --output-file hack/compute/namespace.yaml
	applier render --paths resources/compute-templates/virtual-workspace/service_account.yaml \
	               --values resources/compute-templates/hack-values.yaml --output-file hack/compute/service_account.yaml
	applier render --paths resources/compute-templates/virtual-workspace/role_binding.yaml \
	               --values resources/compute-templates/hack-values.yaml --output-file hack/compute/role_binding.yaml
	applier render --paths resources/compute-templates/virtual-workspace/apiexport.yaml \
	               --values resources/compute-templates/hack-values.yaml --output-file hack/compute/apiexport.yaml

# Generate code
generate: kubebuilder-tools controller-gen register-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

install-prereqs: generate manifests
	kubectl delete secret mce-kubeconfig-secret -n ${POD_NAMESPACE} --ignore-not-found
	kubectl create secret generic mce-kubeconfig-secret -n ${POD_NAMESPACE} --from-file=kubeconfig=${HUB_KUBECONFIG}
	kubectl delete secret kcp-kubeconfig -n ${POD_NAMESPACE} --ignore-not-found
	kubectl create secret generic kcp-kubeconfig -n ${POD_NAMESPACE} --from-file=kubeconfig=${KCP_KUBECONFIG}
	kubectl apply -f config/crd/singapore.open-cluster-management.io_hubconfigs.yaml
	kubectl apply -f config/crd/singapore.open-cluster-management.io_clusterregistrars.yaml
	kubectl apply -f hack/hubconfig.yaml
	kubectl apply -f hack/clusterregistrar.yaml
	kubectl apply -f config/apiresourceschema/singapore.open-cluster-management.io_registeredclusters.yaml --kubeconfig ${KCP_KUBECONFIG}
	kubectl apply -f hack/compute/apiexport.yaml --kubeconfig ${KCP_KUBECONFIG}

run-local: install-prereqs
	go run main.go manager

# Tag the IMG as latest and docker push
docker-push-latest:
	docker tag ${IMG} ${IMAGE_TAG_BASE}:latest
	$(MAKE) docker-push IMG=${IMAGE_TAG_BASE}:latest

.PHONY: build-e2e-test-image
build-e2e-test-image:
	@echo "Building $(IMAGE_E2E_TEST)"
	docker build . \
	-f Dockerfile.cypress \
	-t ${IMG_E2E_TEST}

.PHONY: e2e-ginkgo-test
e2e-ginkgo-test:
	@echo running e2e ginkgo tests
#	ginkgo -tags e2e -v test/e2e -- -v=5
	ginkgo -tags e2e test/e2e
