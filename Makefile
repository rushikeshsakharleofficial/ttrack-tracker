.PHONY: all build test fmt vet install clean

PREFIX ?= /usr/local

all: build

build:
	CGO_ENABLED=0 go build -trimpath -o bin/ttrack ./cmd/ttrack

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

install: build
	install -Dm755 bin/ttrack $(DESTDIR)$(PREFIX)/bin/ttrack

clean:
	rm -rf bin
