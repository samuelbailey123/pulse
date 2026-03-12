VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE      ?= $(shell date -u +%Y-%m-%d)
LDFLAGS    = -s -w \
             -X github.com/samuelbailey123/pulse/cmd.Version=$(VERSION) \
             -X github.com/samuelbailey123/pulse/cmd.Commit=$(COMMIT) \
             -X github.com/samuelbailey123/pulse/cmd.Date=$(DATE)

.PHONY: build test test-race coverage lint install clean

build:
	go build -ldflags "$(LDFLAGS)" -o pulse .

test:
	go test ./...

test-race:
	go test -race ./...

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	@go tool cover -func=coverage.out | grep total | awk '{print $$3}'

lint:
	golangci-lint run ./...

install:
	go install -ldflags "$(LDFLAGS)" .

clean:
	rm -f pulse coverage.out
	rm -rf dist/
