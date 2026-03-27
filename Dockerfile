# +-------------------------------------------------------------------+
# | Copyright IBM Corp. 2025 All Rights Reserved                      |
# | PID 5698-SPR                                                      |
# +-------------------------------------------------------------------+

ARG BUILDER_IMAGE
FROM ${BUILDER_IMAGE:-registry.access.redhat.com/ubi9/go-toolset:9.6-1754467841} AS builder
ARG TARGETOS
ARG TARGETARCH
USER root

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
COPY vendor/ vendor/

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/
COPY assets/ assets/
COPY const/ const/
COPY pkg/ pkg/
COPY internal/ internal/
ARG BUILD_FLAGS=""

ENV GOTOOLCHAIN="go1.24.13"

RUN echo "TARGETARCH = '${TARGETARCH}' TARGETOS='${TARGETOS}'" && \
    echo "GO ENV DUMP: " && go env GOVERSION && go env GOTOOLDIR && \
    CGO_ENABLED=1 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GO111MODULE=on \
    go build ${BUILD_FLAGS} -mod vendor -tags strictfipsruntime -a -o manager main.go


RUN dnf --installroot=/tmp/ubi-micro \
    --nodocs --setopt=install_weak_deps=False \
    install -y \
    shadow-utils openssl-libs openssl-fips-provider && \
    dnf --installroot=/tmp/ubi-micro \
    clean all



FROM registry.access.redhat.com/ubi9-micro:9.6 AS final

ARG VERSION

LABEL io.k8s.display-name="IBM Spyre Operator"
LABEL name="IBM Spyre Operator"
LABEL vendor="IBM"
LABEL maintainer="IBM"
LABEL version="${VERSION}"
LABEL release="N/A"
LABEL summary="Automate the management and monitoring of IBM Spyre devices."
LABEL description="See summary"

WORKDIR /
RUN mkdir -p /opt/spyre-operator

COPY --from=builder /tmp/ubi-micro/ /
RUN useradd -u 1000 spyre-operator

COPY assets /opt/spyre-operator/
RUN chown -R spyre-operator /opt/spyre-operator
RUN chmod -R 755 /opt/spyre-operator
COPY ./LICENSE /licenses/LICENSE

ENV VERSION=${VERSION}
USER spyre-operator

COPY --from=builder /workspace/manager /usr/bin/
HEALTHCHECK NONE
ENTRYPOINT ["/usr/bin/manager"]
