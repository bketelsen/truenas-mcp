BINARY   := truenas-mcp
MODULE   := truenas-mcp
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT   ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS  := -s -w

.PHONY: all build install clean fmt vet lint test tidy run

all: fmt vet build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

install:
	go install -ldflags "$(LDFLAGS)" .

run: build
	./$(BINARY) serve

clean:
	rm -f $(BINARY)
	go clean -cache -testcache

fmt:
	gofmt -s -w .

vet:
	go vet ./...

lint:
	golangci-lint run ./...

test:
	go test -race -count=1 ./...

tidy:
	go mod tidy
	go mod verify
