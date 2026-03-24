.PHONY: build install clean lint test

VERSION ?= dev
LDFLAGS := -ldflags "-s -w -X github.com/tiny-oc/toc/cmd.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o bin/toc .

install:
	go install $(LDFLAGS) .

clean:
	rm -rf bin/

lint:
	go vet ./...

test:
	go test ./...
