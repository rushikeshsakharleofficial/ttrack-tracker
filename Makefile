CC      := gcc
CFLAGS  := -std=c11 -D_GNU_SOURCE -Wall -Wextra -Wformat=2 -Wshadow \
           -O2 -g -fstack-protector-strong -fPIC -D_FORTIFY_SOURCE=2
CFLAGS  += $(CFLAGS_EXTRA)
LDFLAGS := -Wl,-z,relro,-z,now,-z,noexecstack $(LDFLAGS_EXTRA)

# Systemd integration (set to 0 to disable)
HAVE_SYSTEMD ?= 1
ifeq ($(HAVE_SYSTEMD),1)
  CFLAGS  += -DHAVE_SYSTEMD
  SYSTEMD_LIBS := -lsystemd
else
  SYSTEMD_LIBS :=
endif

INCLUDES := -I include

BUILDDIR := build

# Install paths
PREFIX   := /usr
BINDIR   := $(PREFIX)/bin
LIBEXEC  := $(PREFIX)/libexec
SECDIR   := /usr/lib64/security
SYSCONFDIR := /etc/trackterm-rec
STORAGEDIR := /var/lib/trackterm-rec
SYSTEMD_UNIT_DIR := /usr/lib/systemd/system
TMPFILES_DIR := /usr/lib/tmpfiles.d

all: $(BUILDDIR)/trackterm-rec $(BUILDDIR)/trackterm-recd $(BUILDDIR)/trackterm-cli $(BUILDDIR)/pam_record.so

# ─── Common objects ───────────────────────────────────────────────────────────
COMMON_SRC := src/common/log.c src/common/uuid.c src/common/ttyrec.c \
              src/common/proto.c src/common/ringbuf.c
COMMON_OBJ := $(patsubst src/%.c,$(BUILDDIR)/%.o,$(COMMON_SRC))

# ─── Shim (trackterm-rec) ───────────────────────────────────────────────────────────
SHIM_SRC := src/shim/main.c src/shim/pty.c src/shim/loop.c \
            src/shim/session.c src/shim/signals.c \
            src/shim/shell_resolve.c src/shim/env_guard.c
SHIM_OBJ := $(patsubst src/%.c,$(BUILDDIR)/%.o,$(SHIM_SRC))

$(BUILDDIR)/trackterm-rec: $(SHIM_OBJ) $(COMMON_OBJ)
	@mkdir -p $(@D)
	$(CC) $(LDFLAGS) -pie -o $@ $^ -lutil

# ─── Daemon (trackterm-recd) ────────────────────────────────────────────────────────
DAEMON_SRC := src/daemon/main.c src/daemon/server.c src/daemon/session_store.c \
              src/daemon/meta.c src/daemon/paths.c src/daemon/rotate.c \
              src/daemon/config.c
DAEMON_OBJ := $(patsubst src/%.c,$(BUILDDIR)/%.o,$(DAEMON_SRC))

$(BUILDDIR)/trackterm-recd: $(DAEMON_OBJ) $(COMMON_OBJ)
	@mkdir -p $(@D)
	$(CC) $(LDFLAGS) -pie -o $@ $^ $(SYSTEMD_LIBS) -lz -lpthread

# ─── PAM module (pam_record.so) ───────────────────────────────────────────────
PAM_SRC := src/pam/pam_record.c src/pam/pam_env_propagate.c src/pam/pam_marker.c
PAM_OBJ := $(patsubst src/%.c,$(BUILDDIR)/%.o,$(PAM_SRC))

$(BUILDDIR)/pam_record.so: $(PAM_OBJ)
	@mkdir -p $(@D)
	$(CC) -shared -Wl,-soname,pam_record.so -o $@ $^ -lpam

# ─── CLI (trackterm-cli) ────────────────────────────────────────────────────────
CLI_SRC := src/cli/main.c src/cli/cmd_list.c src/cli/cmd_play.c \
           src/cli/cmd_tail.c src/cli/cmd_purge.c src/cli/cmd_tree.c \
           src/cli/cmd_tui.c
CLI_OBJ := $(patsubst src/%.c,$(BUILDDIR)/%.o,$(CLI_SRC))

$(BUILDDIR)/trackterm-cli: $(CLI_OBJ) $(COMMON_OBJ)
	@mkdir -p $(@D)
	$(CC) $(LDFLAGS) -pie -o $@ $^ -lncurses

# ─── Generic compile rule ─────────────────────────────────────────────────────
$(BUILDDIR)/%.o: src/%.c
	@mkdir -p $(@D)
	$(CC) $(CFLAGS) $(INCLUDES) -c -o $@ $<

# ─── Unit tests ───────────────────────────────────────────────────────────────
TEST_SRC := $(wildcard tests/unit/*.c)
TEST_BIN := $(patsubst tests/unit/%.c,$(BUILDDIR)/test_%,$(TEST_SRC))

tests: $(TEST_BIN)
	@for t in $(TEST_BIN); do echo "RUN $$t"; $$t; done

$(BUILDDIR)/test_%: tests/unit/%.c $(COMMON_OBJ)
	@mkdir -p $(@D)
	$(CC) $(CFLAGS) $(INCLUDES) -o $@ $^

# ─── Lint ─────────────────────────────────────────────────────────────────────
lint:
	cppcheck --enable=warning,style,performance --std=c11 \
	         -I include src/ 2>&1 | grep -v "^\(Checking\|$\)"

# ─── Install ──────────────────────────────────────────────────────────────────
install: all
	install -Dm755 $(BUILDDIR)/trackterm-rec     $(DESTDIR)$(LIBEXEC)/trackterm-rec
	install -Dm755 $(BUILDDIR)/trackterm-recd    $(DESTDIR)$(LIBEXEC)/trackterm-recd
	install -Dm755 $(BUILDDIR)/trackterm-cli $(DESTDIR)$(BINDIR)/trackterm-cli
	install -Dm755 $(BUILDDIR)/pam_record.so $(DESTDIR)$(SECDIR)/pam_record.so
	install -Dm644 scripts/systemd/trackterm-recd.service \
	               $(DESTDIR)$(SYSTEMD_UNIT_DIR)/trackterm-recd.service
	install -Dm644 scripts/systemd/trackterm-recd.socket \
	               $(DESTDIR)$(SYSTEMD_UNIT_DIR)/trackterm-recd.socket
	install -Dm644 scripts/systemd/trackterm-rec-purge.service \
	               $(DESTDIR)$(SYSTEMD_UNIT_DIR)/trackterm-rec-purge.service
	install -Dm644 scripts/systemd/trackterm-rec-purge.timer \
	               $(DESTDIR)$(SYSTEMD_UNIT_DIR)/trackterm-rec-purge.timer
	install -Dm644 scripts/tmpfiles.d/trackterm-rec.conf \
	               $(DESTDIR)$(TMPFILES_DIR)/trackterm-rec.conf
	install -Dm644 scripts/profile.d/trackterm-rec.sh \
	               $(DESTDIR)/etc/profile.d/trackterm-rec.sh
	install -Dm644 scripts/sudoers.d/trackterm-rec \
	               $(DESTDIR)/etc/sudoers.d/trackterm-rec
	install -Dm755 scripts/install.sh $(DESTDIR)/usr/share/trackterm-rec/install.sh
	install -d $(DESTDIR)$(SYSCONFDIR)
	if [ ! -f $(DESTDIR)$(SYSCONFDIR)/recd.conf ]; then \
	  install -Dm644 config/recd.conf.sample $(DESTDIR)$(SYSCONFDIR)/recd.conf; fi
	if [ ! -f $(DESTDIR)$(SYSCONFDIR)/shells.allow ]; then \
	  install -Dm644 config/shells.allow.sample $(DESTDIR)$(SYSCONFDIR)/shells.allow; fi
	install -d $(DESTDIR)$(STORAGEDIR)
	chown root:root $(DESTDIR)$(STORAGEDIR) 2>/dev/null || true
	chmod 750 $(DESTDIR)$(STORAGEDIR) 2>/dev/null || true

# ─── Uninstall ────────────────────────────────────────────────────────────────
uninstall:
	rm -f $(DESTDIR)$(LIBEXEC)/trackterm-rec
	rm -f $(DESTDIR)$(LIBEXEC)/trackterm-recd
	rm -f $(DESTDIR)$(BINDIR)/trackterm-cli
	rm -f $(DESTDIR)$(SECDIR)/pam_record.so
	rm -f $(DESTDIR)$(SYSTEMD_UNIT_DIR)/trackterm-recd.{service,socket}
	rm -f $(DESTDIR)$(SYSTEMD_UNIT_DIR)/trackterm-rec-purge.{service,timer}
	rm -f $(DESTDIR)$(TMPFILES_DIR)/trackterm-rec.conf
	rm -f $(DESTDIR)/etc/profile.d/trackterm-rec.sh
	rm -f $(DESTDIR)/etc/sudoers.d/trackterm-rec

# ─── RPM package ──────────────────────────────────────────────────────────────
VERSION_STR := $(shell cat VERSION | tr -d '[:space:]')
TARBALL     := $(HOME)/rpmbuild/SOURCES/trackterm-rec-$(VERSION_STR).tar.gz

rpm: all
	@command -v rpmbuild >/dev/null || { echo "rpmbuild not found — install rpm-build"; exit 1; }
	rpmdev-setuptree 2>/dev/null || mkdir -p $(HOME)/rpmbuild/{BUILD,BUILDROOT,RPMS,SOURCES,SPECS,SRPMS}
	cp packaging/rpm/trackterm-rec.spec $(HOME)/rpmbuild/SPECS/trackterm-rec.spec
	cd .. && tar czf $(TARBALL) \
	    --transform 's,^terminal-recorder,trackterm-rec-$(VERSION_STR),' \
	    --exclude 'terminal-recorder/build' \
	    --exclude 'terminal-recorder/.claude' \
	    --exclude 'terminal-recorder/release' \
	    terminal-recorder/
	rpmbuild -ba $(HOME)/rpmbuild/SPECS/trackterm-rec.spec
	mkdir -p release
	cp $(HOME)/rpmbuild/RPMS/x86_64/trackterm-rec-$(VERSION_STR)-*.rpm release/ 2>/dev/null || true
	cp $(HOME)/rpmbuild/RPMS/noarch/trackterm-rec-$(VERSION_STR)-*.rpm  release/ 2>/dev/null || true
	cp $(HOME)/rpmbuild/SRPMS/trackterm-rec-$(VERSION_STR)-*.src.rpm    release/
	@echo "RPM built: $$(ls release/*.rpm | grep -v debuginfo | grep -v debugsource | grep -v src)"

# ─── DEB package ──────────────────────────────────────────────────────────────
deb: all
	bash packaging/deb/build-deb.sh release
	@echo "DEB built: release/trackterm-rec_$(VERSION_STR)-1_amd64.deb"

# ─── Both packages ────────────────────────────────────────────────────────────
packages: rpm deb
	@echo "==> Packages in release/:"
	@ls -lh release/*.rpm release/*.deb 2>/dev/null | grep -v debuginfo | grep -v debugsource

clean:
	rm -rf $(BUILDDIR) release/

.PHONY: all tests lint install uninstall clean rpm deb packages
