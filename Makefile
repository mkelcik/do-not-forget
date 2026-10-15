SHELL := /bin/sh

TOOLBIN := $(shell go env GOPATH)/bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//' || echo dev)

.PHONY: help run build fmt fmt-check vet lint test vuln tidy-check tools ci dist clean

help:
	@printf '%s\n' \
		"Targets:" \
		"  run         Build and run with the dummy event source" \
		"  build       Build ./do-not-forget for this machine" \
		"  fmt         Format Go files" \
		"  fmt-check   Fail if gofmt would change anything" \
		"  vet         go vet" \
		"  lint        golangci-lint" \
		"  test        Tests with the race detector" \
		"  vuln        govulncheck" \
		"  tidy-check  Fail if go mod tidy would change go.mod/go.sum" \
		"  tools       Install pinned golangci-lint and govulncheck (re-run after a Go upgrade)" \
		"  ci          Everything CI runs" \
		"  dist        Release archive for this OS/arch into dist/"

run: build
	./do-not-forget -dummy dummy.json

build:
	go build -ldflags "-X main.version=dev" -o do-not-forget .

fmt:
	gofmt -w .

fmt-check:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

vet:
	go vet ./...

lint:
	$(TOOLBIN)/golangci-lint run ./...

test:
	go test -race -count=1 ./...

vuln:
	$(TOOLBIN)/govulncheck ./...

tidy-check:
	go mod tidy -diff

tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0
	go install golang.org/x/vuln/cmd/govulncheck@v1.9.0

ci: fmt-check tidy-check vet lint test vuln

dist:
	VERSION=$(VERSION) ./scripts/dist.sh

clean:
	rm -rf do-not-forget dist build
