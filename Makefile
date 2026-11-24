BINARY  := omada2pihole
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
IMAGE   ?= omada2pihole:dev

.PHONY: all fmt lint test build docker clean

all: lint test build

fmt: ## Format the code
	gofmt -w .

lint: ## gofmt check + go vet
	@test -z "$$(gofmt -l .)" || (gofmt -l . && echo "run 'make fmt'" && exit 1)
	go vet ./...

test: ## Run unit tests with the race detector
	go test -race -count=1 ./...

build: ## Build a static binary into ./bin
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" -o bin/$(BINARY) ./cmd/$(BINARY)

docker: ## Build the container image
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE) .

clean: ## Remove build output
	go clean
	$(RM) -r bin dist
