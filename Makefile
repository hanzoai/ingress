VERSION := $(shell git describe --tags --always)
BIN := hanzo-ingress
REGISTRY := ghcr.io/hanzoai/ingress

build:
	CGO_ENABLED=0 go build -ldflags="-w -s -X github.com/traefik/traefik/v3/pkg/version.Version=$(VERSION)" -o $(BIN) ./cmd/traefik

build-dist: ## Build for Docker (linux/amd64)
	mkdir -p dist/linux/amd64
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
		-ldflags="-w -s -X github.com/traefik/traefik/v3/pkg/version.Version=$(VERSION)" \
		-o dist/linux/amd64/$(BIN) ./cmd/traefik

docker: build-dist ## Build Docker image
	docker build --platform linux/amd64 \
		-t $(REGISTRY):latest \
		-t $(REGISTRY):$(VERSION) .

docker-push: docker ## Build and push Docker image
	docker push $(REGISTRY):latest
	docker push $(REGISTRY):$(VERSION)

clean:
	rm -f $(BIN)
	rm -rf dist/

.PHONY: build build-dist docker docker-push clean
