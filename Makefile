.PHONY: all build test fmt vet install clean rpm deb packages

PREFIX ?= /usr/local
VERSION ?= 0.4.11
NFPM ?= $(shell go env GOPATH)/bin/nfpm

all: build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-X main.Version=$(VERSION)" -o bin/ttrack ./cmd/ttrack
	CGO_ENABLED=0 go build -trimpath -o bin/ttrackd ./cmd/ttrackd

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

install: build
	install -Dm755 bin/ttrack $(DESTDIR)$(PREFIX)/bin/ttrack
	install -Dm755 bin/ttrackd $(DESTDIR)/usr/libexec/ttrackd
	install -Dm644 man/ttrack.1 $(DESTDIR)$(PREFIX)/share/man/man1/ttrack.1
	install -Dm644 scripts/systemd/ttrackd.service $(DESTDIR)/lib/systemd/system/ttrackd.service
	install -Dm644 internal/complete/ttrack.bash $(DESTDIR)$(PREFIX)/share/bash-completion/completions/ttrack
	install -Dm755 scripts/ttrack-ssh-wrap.sh $(DESTDIR)/usr/libexec/ttrack-ssh-wrap
	install -Dm644 scripts/sshd-forcecommand.conf.example $(DESTDIR)$(PREFIX)/share/doc/ttrack/sshd-forcecommand.conf.example
	install -dm700 $(DESTDIR)/var/lib/ttrack

packages: rpm deb

rpm: build
	@mkdir -p release
	TTRACK_VERSION=$(VERSION) $(NFPM) pkg --config nfpm.yaml --packager rpm --target release/

deb: build
	@mkdir -p release
	TTRACK_VERSION=$(VERSION) $(NFPM) pkg --config nfpm.yaml --packager deb --target release/

clean:
	rm -rf bin release
