BINARY := debrief
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build test lint clean install run

build:
	go build $(LDFLAGS) -o bin/$(BINARY) ./cmd/debrief

install:
	go install $(LDFLAGS) ./cmd/debrief

test:
	go test -race -count=1 ./...

lint:
	go vet ./...
	golangci-lint run

clean:
	rm -rf bin/ dist/

run: build
	./bin/$(BINARY)
