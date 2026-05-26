.PHONY: all build test fmt vet install clean rpm deb packages

PREFIX ?= /usr/local
VERSION ?= 0.1.0
NFPM ?= $(shell go env GOPATH)/bin/nfpm

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
	install -Dm644 man/ttrack.1 $(DESTDIR)$(PREFIX)/share/man/man1/ttrack.1

packages: rpm deb

rpm: build
	@mkdir -p release
	TTRACK_VERSION=$(VERSION) $(NFPM) pkg --config nfpm.yaml --packager rpm --target release/

deb: build
	@mkdir -p release
	TTRACK_VERSION=$(VERSION) $(NFPM) pkg --config nfpm.yaml --packager deb --target release/

clean:
	rm -rf bin release
