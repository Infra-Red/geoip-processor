# Intermediate image to build binary
FROM --platform=${BUILDPLATFORM:-linux/amd64} golang:1.24-alpine@sha256:d9db32125db0c3a680cfb7a1afcaefb89c898a075ec148fdc2f0f646cc2ed509 AS builder

ARG TARGETPLATFORM
ARG BUILDPLATFORM
ARG TARGETOS
ARG TARGETARCH

ARG GEOIP_CDN_ENDPOINT

WORKDIR /builder
COPY . .
RUN \
    ./update_geoip2.py -c ${GEOIP_CDN_ENDPOINT} -d GeoIP2-Country -o cmd/geoip-processor && \
    ./update_geoip2.py -c ${GEOIP_CDN_ENDPOINT} -d GeoIP2-City -o cmd/geoip-processor
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o geoip-processor ./cmd/geoip-processor

# Final image
# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static-debian12:nonroot@sha256:c0f429e16b13e583da7e5a6ec20dd656d325d88e6819cafe0adb0828976529dc AS final
COPY --from=builder --chown=nonroot:nonroot /builder/geoip-processor /usr/bin/
USER 65532:65532
ENTRYPOINT ["geoip-processor"]