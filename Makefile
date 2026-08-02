BINARY     := model-advisor
MODULE     := ./cmd/server

VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE       ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo "unknown")

LDFLAGS    := -s -w \
              -X main.version=$(VERSION) \
              -X main.commit=$(COMMIT) \
              -X main.date=$(DATE)

.PHONY: all build test lint install cross-compile release-dryrun clean fmt tidy

all: fmt tidy lint test build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(MODULE)

test:
ifeq ($(shell CGO_ENABLED=1 go env CGO_ENABLED 2>/dev/null),1)
	go test -race -cover ./...
else
	go test -cover ./...
endif

lint:
	go vet ./...

install:
	go install -ldflags "$(LDFLAGS)" $(MODULE)

cross-compile:
	GOOS=linux   GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINARY)-linux-amd64   $(MODULE)
	GOOS=linux   GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BINARY)-linux-arm64   $(MODULE)
	GOOS=darwin  GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINARY)-darwin-amd64  $(MODULE)
	GOOS=darwin  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BINARY)-darwin-arm64  $(MODULE)
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINARY)-windows-amd64.exe $(MODULE)

release-dryrun:
	goreleaser release --snapshot --clean

clean:
	rm -f $(BINARY) $(BINARY)-linux-amd64 $(BINARY)-linux-arm64 \
	      $(BINARY)-darwin-amd64 $(BINARY)-darwin-arm64 \
	      $(BINARY)-windows-amd64.exe

fmt:
	gofmt -w .

tidy:
	go mod tidy
