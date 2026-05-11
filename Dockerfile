# Build the manager binary
#
# Pin to BUILDPLATFORM so the builder stage runs natively on the runner's
# architecture, and use Go's GOOS/GOARCH cross-compilation instead of QEMU.
# This makes multi-arch builds ~10x faster (arm64 buildx via QEMU normally
# takes 15-20 min; cross-compile finishes in <2 min).
FROM --platform=$BUILDPLATFORM golang:1.25 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the Go source (relies on .dockerignore to filter)
COPY . .

# Build
#   -trimpath: deterministic build (strip absolute paths from binary)
#   -ldflags="-s -w": strip DWARF debug + symbol tables (~25% smaller binary)
# CGO disabled so the binary is statically linked → works on distroless/static.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -a -o manager cmd/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
