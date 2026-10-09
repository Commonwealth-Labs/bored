.PHONY: build install install-skills test vet fmt smoke clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-X github.com/Commonwealth-Labs/bored/internal/cli.Version=$(VERSION)"

build:
	go build $(LDFLAGS) -o bored ./cmd/bored

install:
	go install $(LDFLAGS) ./cmd/bored

install-skills: install
	bored install-skills --force

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

smoke: build
	BORED_BIN=$(CURDIR)/bored ./scripts/smoke.sh

clean:
	rm -f bored
