IMAGE_NAME?=geoip-processor

.PHONY: test
test:
	@go test ./...

.PHONY: run
run:
	@go run ./cmd/geoip-processor

clean:
	@rm -rf cmd/geoip-processor/*.mmdb

.PHONY: build
build: clean
	@docker build -t ${IMAGE_NAME} --build-arg GEOIP_CDN_ENDPOINT=${GEOIP_CDN_ENDPOINT} --build-arg TARGETPLATFORM=linux/amd64 --build-arg TARGETOS=linux --build-arg TARGETARCH=amd64 .
