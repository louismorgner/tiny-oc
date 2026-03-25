.PHONY: build install clean lint test test-e2e

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

test-e2e:
	go test ./e2e/ -v -count=1 -timeout 120s
