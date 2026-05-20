#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <fcntl.h>
#include <errno.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <sys/stat.h>
#include <sys/epoll.h>

#include <grp.h>
#include "pmp_proto.h"
#include "pmp_log.h"
#include "pmp_paths.h"

/* Forward */
typedef struct pmp_session_store pmp_session_store_t;
pmp_session_store_t *pmp_store_open(const char *storage_dir,
                                    const struct pmp_hello *h);
int  pmp_store_write_output(pmp_session_store_t *s,
                            const uint8_t *data, uint32_t len);
int  pmp_store_write_resize(pmp_session_store_t *s, double t,
                            uint16_t rows, uint16_t cols);
void pmp_store_close(pmp_session_store_t *s, int exit_status);

typedef struct pmp_client {
    int                  fd;
    uid_t                peer_uid;
    gid_t                peer_gid;
    pid_t                peer_pid;
    uint32_t             loginuid;
    char                 sid[37];
    struct frame_parser  parser;
    pmp_session_store_t *store;
    uint64_t             session_start_ns;
    int                  got_hello;
} pmp_client_t;

#define MAX_CLIENTS 256

static pmp_client_t *g_clients[MAX_CLIENTS];
static int g_ncli = 0;
static const char *g_storage_dir = NULL;

int pmp_server_bind(const char *sockpath, mode_t mode)
{
    struct sockaddr_un addr;
    int fd;

    unlink(sockpath);

    fd = socket(AF_UNIX, SOCK_STREAM | SOCK_NONBLOCK | SOCK_CLOEXEC, 0);
    if (fd < 0) return -1;

    memset(&addr, 0, sizeof(addr));
    addr.sun_family = AF_UNIX;
    strncpy(addr.sun_path, sockpath, sizeof(addr.sun_path) - 1);

    if (bind(fd, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
        close(fd);
        return -1;
    }

    chmod(sockpath, mode);

    /* Set group ownership to pmp-audit so mode 0660 restricts access correctly.
     * Only processes running with gid=pmp-audit (via setgid bit on pmp-rec) connect. */
    struct group *gr = getgrnam("pmp-audit");
    if (gr)
        chown(sockpath, (uid_t)-1, gr->gr_gid);

    if (listen(fd, 64) < 0) {
        close(fd);
        return -1;
    }

    return fd;
}

/* Validate that s is a well-formed UUID: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
 * Rejects any path traversal sequences (../, /, etc.) before use in file paths. */
static int is_valid_uuid(const char *s)
{
    static const int dash_pos[] = {8, 13, 18, 23};
    if (!s || strlen(s) != 36) return 0;
    for (int i = 0; i < 36; i++) {
        if (i == 8 || i == 13 || i == 18 || i == 23) {
            if (s[i] != '-') return 0;
        } else {
            char c = s[i];
            if (!((c >= '0' && c <= '9') ||
                  (c >= 'a' && c <= 'f') ||
                  (c >= 'A' && c <= 'F'))) return 0;
        }
    }
    (void)dash_pos;
    return 1;
}

static uint32_t read_loginuid_for_pid(pid_t pid)
{
    char path[64];
    char buf[32];
    snprintf(path, sizeof(path), "/proc/%d/loginuid", pid);
    int fd = open(path, O_RDONLY | O_CLOEXEC);
    if (fd < 0) return 0xFFFFFFFFu;
    ssize_t n = read(fd, buf, sizeof(buf)-1);
    close(fd);
    if (n <= 0) return 0xFFFFFFFFu;
    buf[n] = '\0';
    return (uint32_t)strtoul(buf, NULL, 10);
}

int pmp_server_accept(int listen_fd, const char *storage_dir, int epoll_fd)
{
    struct sockaddr_un addr;
    socklen_t addrlen = sizeof(addr);
    int fd;

    fd = accept4(listen_fd, (struct sockaddr *)&addr, &addrlen,
                 SOCK_NONBLOCK | SOCK_CLOEXEC);
    if (fd < 0) {
        if (errno == EAGAIN || errno == EWOULDBLOCK) return 0;
        return -1;
    }

    if (g_ncli >= MAX_CLIENTS) {
        PMP_LOG_WARN("client limit reached, dropping");
        close(fd);
        return 0;
    }

    /* SO_PEERCRED */
    struct ucred cred;
    socklen_t credlen = sizeof(cred);
    if (getsockopt(fd, SOL_SOCKET, SO_PEERCRED, &cred, &credlen) < 0) {
        PMP_LOG_WARN("SO_PEERCRED failed: %s", strerror(errno));
        close(fd);
        return 0;
    }

    pmp_client_t *c = calloc(1, sizeof(*c));
    if (!c) { close(fd); return -1; }

    c->fd       = fd;
    c->peer_uid = cred.uid;
    c->peer_gid = cred.gid;
    c->peer_pid = cred.pid;
    c->loginuid = read_loginuid_for_pid(cred.pid);

    frame_parser_init(&c->parser);
    g_clients[g_ncli++] = c;

    if (!g_storage_dir) g_storage_dir = storage_dir;

    /* Add to epoll */
    struct epoll_event ev;
    ev.events  = EPOLLIN | EPOLLHUP | EPOLLERR;
    ev.data.ptr = c;
    epoll_ctl(epoll_fd, EPOLL_CTL_ADD, fd, &ev);

    PMP_LOG_DBG("client accepted uid=%u pid=%d", cred.uid, cred.pid);
    return 0;
}

static void close_client(pmp_client_t *c, int exit_status, int epoll_fd)
{
    if (!c) return;

    if (c->store) {
        pmp_store_close(c->store, exit_status);
        c->store = NULL;
    }

    frame_parser_free(&c->parser);

    if (epoll_fd >= 0)
        epoll_ctl(epoll_fd, EPOLL_CTL_DEL, c->fd, NULL);
    close(c->fd);

    /* Remove from g_clients */
    for (int i = 0; i < g_ncli; i++) {
        if (g_clients[i] == c) {
            g_clients[i] = g_clients[--g_ncli];
            break;
        }
    }
    free(c);
}

static int handle_frame(pmp_client_t *c, const struct pmp_frame *f)
{
    double t_sec = 0.0;

    if (c->session_start_ns)
        t_sec = (double)(f->hdr.ts_ns - c->session_start_ns) / 1e9;

    switch (f->hdr.type) {
    case PMP_F_HELLO: {
        if (f->hdr.payload_len < sizeof(struct pmp_hello)) {
            PMP_LOG_WARN("short hello from pid=%d", c->peer_pid);
            return -1;
        }
        const struct pmp_hello *h = (const struct pmp_hello *)f->payload;

        /* Cross-check: hello loginuid must match kernel loginuid */
        if (c->loginuid != 0xFFFFFFFFu && h->loginuid != c->loginuid &&
            c->peer_uid != 0) {
            PMP_LOG_WARN("loginuid mismatch: claimed=%u kernel=%u",
                         h->loginuid, c->loginuid);
            return -1;
        }

        /* Reject non-UUID SIDs — prevents path traversal in pmp_paths_build */
        if (!is_valid_uuid(h->sid)) {
            PMP_LOG_WARN("invalid SID from pid=%d, rejecting", c->peer_pid);
            return -1;
        }
        if (h->parent_sid[0] && !is_valid_uuid(h->parent_sid)) {
            PMP_LOG_WARN("invalid parent_sid from pid=%d, rejecting", c->peer_pid);
            return -1;
        }

        strncpy(c->sid, h->sid, 36);
        c->session_start_ns = f->hdr.ts_ns;
        c->got_hello = 1;

        c->store = pmp_store_open(g_storage_dir, h);
        if (!c->store) PMP_LOG_WARN("store_open failed for %s", h->sid);
        break;
    }
    case PMP_F_OUT:
        if (!c->got_hello || !c->store) break;
        pmp_store_write_output(c->store,
                               (const uint8_t *)f->payload,
                               f->hdr.payload_len);
        break;

    case PMP_F_RESIZE: {
        if (!c->got_hello || !c->store) break;
        if (f->hdr.payload_len < sizeof(struct pmp_resize)) break;
        const struct pmp_resize *rs = (const struct pmp_resize *)f->payload;
        pmp_store_write_resize(c->store, t_sec, rs->rows, rs->cols);
        break;
    }
    case PMP_F_CLOSE: {
        int exit_status = 0;
        if (f->hdr.payload_len >= 4) {
            int32_t s;
            memcpy(&s, f->payload, 4);
            exit_status = (int)s;
        }
        return -(exit_status + 1000); /* signal caller to close */
    }
    case PMP_F_HEARTBEAT:
        break;
    default:
        PMP_LOG_DBG("unknown frame type %u", f->hdr.type);
    }
    return 0;
}

int pmp_server_handle_client(pmp_client_t *c, int epoll_fd)
{
    uint8_t buf[65536 + sizeof(struct pmp_frame_hdr)];
    ssize_t n;

    for (;;) {
        n = read(c->fd, buf, sizeof(buf));
        if (n < 0) {
            if (errno == EAGAIN || errno == EWOULDBLOCK) return 0;
            close_client(c, -1, epoll_fd);
            return -1;
        }
        if (n == 0) {
            close_client(c, 0, epoll_fd);
            return -1;
        }

        size_t pos = 0;
        while (pos < (size_t)n) {
            struct pmp_frame f;
            size_t consumed;
            int r = frame_parser_feed(&c->parser, buf + pos, (size_t)n - pos,
                                      &consumed, &f);
            pos += consumed;
            if (r < 0) {
                PMP_LOG_WARN("parse error from pid=%d: %d", c->peer_pid, r);
                frame_parser_reset(&c->parser);
                close_client(c, -1, epoll_fd);
                return -1;
            }
            if (r == 1) {
                int hr = handle_frame(c, &f);
                free((void *)f.payload);

                if (hr < -1) {
                    int exit_code = -(hr + 1000);
                    close_client(c, exit_code, epoll_fd);
                    return -1;
                } else if (hr < 0) {
                    close_client(c, -1, epoll_fd);
                    return -1;
                }
            }
        }
    }
}

/* Called when epoll reports EPOLLHUP/EPOLLERR */
void pmp_server_drop_client(pmp_client_t *c, int epoll_fd)
{
    close_client(c, -1, epoll_fd);
}
