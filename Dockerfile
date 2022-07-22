
# Copyright Red Hat

FROM registry.ci.openshift.org/stolostron/builder:go1.18-linux AS builder

WORKDIR /workspace

COPY go.mod go.mod
COPY go.sum go.sum
# uncomment the following COPY and comment the `COPY go mod download` if you are replacing module in the go.mod by a local directory
# you will need to run `go mod vendor` prior building the image.
# COPY vendor vendor
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

COPY main.go main.go
COPY api/ api/
# COPY cmd/ cmd/
COPY deploy/ deploy/
COPY resources/ resources/
COPY pkg/ pkg/

COPY config/apiresourceschema config/apiresourceschema
COPY config/rbac config/rbac
COPY config/crd config/crd
COPY config/resources.go config/resources.go
COPY controllers/ controllers/
COPY webhook/ webhook/

RUN GOFLAGS="" go build -a -o compute-operator main.go

COPY config/ config/
COPY build/bin/ build/bin/

FROM registry.access.redhat.com/ubi8/ubi-minimal:latest
RUN microdnf update

ENV OPERATOR=/usr/local/bin/compute-operator \
    USER_UID=1001 \
    USER_NAME=compute-operator

COPY --from=builder /workspace/compute-operator ${OPERATOR}
COPY --from=builder /workspace/build/bin /usr/local/bin

RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}
