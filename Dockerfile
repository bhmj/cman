ARG GOLANG_VERSION=1.24
ARG GOLANG_IMAGE=alpine
ARG TARGET_DISTR_TYPE=alpine
ARG TARGET_DISTR_VERSION=latest

# -- Build stage --
FROM golang:${GOLANG_VERSION}-${GOLANG_IMAGE} AS builder

ARG LDFLAGS
ARG GOOS=linux
ARG GOARCH=amd64

WORKDIR /source
COPY go.mod go.sum ./
COPY vendor/ vendor/
COPY cmd/ cmd/
COPY internal/ internal/
RUN GOOS=$GOOS GOARCH=$GOARCH CGO_ENABLED=0 go build -ldflags "$LDFLAGS" -trimpath -o bin/cman ./cmd/

# -- Runtime stage --
ARG TARGET_DISTR_TYPE
ARG TARGET_DISTR_VERSION
FROM ${TARGET_DISTR_TYPE}:${TARGET_DISTR_VERSION} AS cman
ARG APP_USER=dummy
RUN apk add --no-cache bash docker-cli && \
    addgroup -g 1337 -S ${APP_USER} && \
    adduser -u 1337 -G ${APP_USER} -S ${APP_USER}
WORKDIR /app
COPY --from=builder /source/bin/cman .
ENTRYPOINT ["./cman"]

# -- k8s image: includes config and docker-assets for building playground images inside DinD --
FROM cman AS cman-k8s
COPY config/ /app/config/
COPY docker-assets/ /app/docker-assets/
COPY k3s/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh && mkdir -p /data/playground
EXPOSE 8260 8261
ENTRYPOINT ["/app/entrypoint.sh"]
