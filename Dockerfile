# syntax=docker/dockerfile:1

# The build stage runs on the build host's platform and cross-compiles, so
# multi-arch images build without emulation.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -buildvcs=false -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/omada2pihole ./cmd/omada2pihole

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
LABEL org.opencontainers.image.source="https://github.com/filipegalo/omada2pihole" \
      org.opencontainers.image.description="Sync TP-Link Omada client names into Pi-hole v6 local DNS records" \
      org.opencontainers.image.licenses="MIT"
COPY --from=build /out/omada2pihole /omada2pihole
USER 65532:65532
ENTRYPOINT ["/omada2pihole"]
