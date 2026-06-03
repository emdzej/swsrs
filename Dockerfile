# syntax=docker/dockerfile:1.7

# --- build stage ---
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev

# Cache modules separately from source for faster rebuilds.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Static, stripped binary. CGO disabled so it runs in distroless/static.
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=$VERSION" \
    -o /out/swsrs ./cmd/swsrs

# --- runtime stage ---
# distroless/static-debian12 ships CA certs (needed for OIDC discovery + admin
# clients calling https) and runs as nonroot UID 65532. No shell, no package
# manager — small attack surface.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/swsrs /swsrs

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/swsrs"]
CMD ["serve"]
