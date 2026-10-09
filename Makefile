.PHONY: build install test vet fmt smoke clean

build:
	go build -o bored ./cmd/bored

install:
	go install ./cmd/bored

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
