# syntax=docker/dockerfile:1.4

############################
# 2) Builder stage
############################
FROM --platform=$BUILDPLATFORM golang:1.24.4 AS builder

WORKDIR /workspace

# 2.1) Cache and download dependencies first
COPY go.mod go.sum ./
RUN go mod download

# 2.2) Copy only the source directories needed
COPY pkg ./pkg
COPY cmd ./cmd

# 2.3) Cross-compile for each TARGETOS/TARGETARCH
ARG TARGETOS TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
     CGO_ENABLED=0 GO111MODULE=on GOOS=$TARGETOS GOARCH=$TARGETARCH \
      go build \
        -trimpath -a \
        -ldflags="-s -w" \
        -o karpenter \
        ./cmd/controller/main.go

RUN ls -lhast

############################
# 3) Runtime stage
############################
FROM gcr.io/distroless/static:nonroot AS runner

# Copy the statically linked binary
COPY --from=builder /workspace/karpenter /usr/local/bin/karpenter

# Run as non-root user (distroless nonroot is UID 65532)
USER nonroot

ENTRYPOINT ["/usr/local/bin/karpenter"]