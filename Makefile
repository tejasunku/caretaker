BINARY  := bin/caretaker
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/tejasunku/caretaker/internal/cli.Version=$(VERSION)

.PHONY: setup build test test-integration fmt vet lint run clean

setup:
	./scripts/setup.sh

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/caretaker

test:
	go test ./...

test-integration:
	go test -tags integration -count=1 ./test/integration/...

fmt:
	gofmt -w ./cmd ./internal

vet:
	go vet ./...

lint: fmt vet

run: build
	./$(BINARY) --help

clean:
	rm -f $(BINARY)
