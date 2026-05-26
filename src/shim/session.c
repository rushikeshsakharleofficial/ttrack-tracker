#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <fcntl.h>
#include <errno.h>
#include <sys/socket.h>
#include <sys/un.h>
#include <sys/ioctl.h>
#include <pwd.h>
#include <time.h>

#include "trackterm_proto.h"
#include "trackterm_paths.h"
#include "trackterm_uuid.h"
#include "trackterm_log.h"
#include "trackterm_compat.h"

/* Defined in loop.c / main.c for the shim context */
typedef struct trackterm_shim_ctx trackterm_shim_ctx_t;
struct trackterm_shim_ctx;

uint32_t trackterm_session_read_loginuid(void)
{
    char buf[32];
    int fd;
    ssize_t n;
    uint32_t val;

    fd = open(TRACKTERM_LOGINUID_PATH, O_RDONLY | O_CLOEXEC);
    if (fd < 0) return TRACKTERM_LOGINUID_UNSET;

    n = read(fd, buf, sizeof(buf) - 1);
    close(fd);

    if (n <= 0) return TRACKTERM_LOGINUID_UNSET;
    buf[n] = '\0';

    val = (uint32_t)strtoul(buf, NULL, 10);
    return val;
}

int trackterm_session_connect_daemon(int fail_closed)
{
    struct sockaddr_un addr;
    int fd, i;
    int tries = fail_closed ? 5 : 3;

    for (i = 0; i < tries; i++) {
        fd = socket(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0);
        if (fd < 0) return -1;

        memset(&addr, 0, sizeof(addr));
        addr.sun_family = AF_UNIX;
        strncpy(addr.sun_path, TRACKTERM_SOCK_PATH, sizeof(addr.sun_path) - 1);

        if (connect(fd, (struct sockaddr *)&addr, sizeof(addr)) == 0)
            return fd;

        close(fd);

        if (i < tries - 1) {
            struct timespec ts = { .tv_sec = 0, .tv_nsec = 200000000L }; /* 200ms */
            nanosleep(&ts, NULL);
        }
    }

    if (fail_closed)
        TRACKTERM_LOG_ERR("trackterm-recd unreachable at %s — session denied (fail_closed)", TRACKTERM_SOCK_PATH);
    else
        TRACKTERM_LOG_WARN("trackterm-recd unreachable at %s — recording disabled", TRACKTERM_SOCK_PATH);

    return -1;
}

static ssize_t write_all(int fd, const void *buf, size_t n)
{
    const uint8_t *p = buf;
    while (n > 0) {
        ssize_t r = write(fd, p, n);
        if (r < 0) {
            if (errno == EINTR) continue;
            return -1;
        }
        p += r;
        n -= (size_t)r;
    }
    return 0;
}

int trackterm_session_send_hello(int daemon_fd,
                           const char *sid, const char *parent_sid,
                           uint32_t loginuid, const char *service,
                           const char *tty, uint16_t rows, uint16_t cols,
                           const char *rhost)
{
    struct trackterm_hello hello;
    uint8_t buf[sizeof(struct trackterm_frame_hdr) + sizeof(hello)];
    struct passwd *pw;
    char hostname[64];
    ssize_t n;

    memset(&hello, 0, sizeof(hello));
    strncpy(hello.sid,        sid,        36);
    strncpy(hello.parent_sid, parent_sid ? parent_sid : "", 36);
    hello.loginuid = loginuid;
    hello.euid     = (uint32_t)geteuid();
    hello.gid      = (uint32_t)getgid();
    hello.rows     = rows;
    hello.cols     = cols;
    strncpy(hello.service, service ? service : "unknown", sizeof(hello.service) - 1);
    strncpy(hello.tty,     tty     ? tty     : "",        sizeof(hello.tty) - 1);
    strncpy(hello.term,    getenv("TERM") ? getenv("TERM") : "xterm",
            sizeof(hello.term) - 1);
    strncpy(hello.rhost,   rhost   ? rhost   : "",        sizeof(hello.rhost) - 1);

    pw = getpwuid(getuid());
    strncpy(hello.ruser, pw ? pw->pw_name : "", sizeof(hello.ruser) - 1);

    gethostname(hostname, sizeof(hostname));
    strncpy(hello.hostname, hostname, sizeof(hello.hostname) - 1);

    n = frame_encode(buf, sizeof(buf),
                     TRACKTERM_F_HELLO,
                     trackterm_clock_ns(), 0,
                     &hello, sizeof(hello),
                     1 /* include_magic */);
    if (n < 0) return -1;

    return (int)write_all(daemon_fd, buf, (size_t)n);
}

int trackterm_session_send_close(int daemon_fd, uint64_t *seq_ptr, int exit_code)
{
    struct trackterm_close cl;
    uint8_t buf[sizeof(struct trackterm_frame_hdr) + sizeof(cl)];
    ssize_t n;

    cl.exit_status = (int32_t)exit_code;

    n = frame_encode(buf, sizeof(buf),
                     TRACKTERM_F_CLOSE,
                     trackterm_clock_ns(), (*seq_ptr)++,
                     &cl, sizeof(cl),
                     0);
    if (n < 0) return -1;

    return (int)write_all(daemon_fd, buf, (size_t)n);
}

int trackterm_session_send_resize(int daemon_fd, uint64_t *seq_ptr,
                            uint16_t rows, uint16_t cols)
{
    struct trackterm_resize rs;
    uint8_t buf[sizeof(struct trackterm_frame_hdr) + sizeof(rs)];
    ssize_t n;

    rs.rows = rows; rs.cols = cols; rs.xpixel = 0; rs.ypixel = 0;

    n = frame_encode(buf, sizeof(buf),
                     TRACKTERM_F_RESIZE,
                     trackterm_clock_ns(), (*seq_ptr)++,
                     &rs, sizeof(rs),
                     0);
    if (n < 0) return -1;

    return write_all(daemon_fd, buf, (size_t)n);
}

int trackterm_session_send_out(int daemon_fd, uint64_t *seq_ptr,
                         const uint8_t *data, uint32_t len)
{
    uint8_t buf[sizeof(struct trackterm_frame_hdr)];
    uint8_t hdr_and_data[sizeof(struct trackterm_frame_hdr) + 65536];
    ssize_t n;

    if (len > 65536) len = 65536;

    n = frame_encode(hdr_and_data, sizeof(hdr_and_data),
                     TRACKTERM_F_OUT,
                     trackterm_clock_ns(), (*seq_ptr)++,
                     data, len,
                     0);
    if (n < 0) return -1;

    (void)buf;
    return write_all(daemon_fd, hdr_and_data, (size_t)n);
}

int trackterm_session_send_gap(int daemon_fd, uint64_t *seq_ptr, double gap_seconds)
{
    struct trackterm_gap g;
    uint8_t buf[sizeof(struct trackterm_frame_hdr) + sizeof(g)];
    ssize_t n;

    g.gap_seconds = gap_seconds;
    n = frame_encode(buf, sizeof(buf),
                     TRACKTERM_F_GAP,
                     trackterm_clock_ns(), (*seq_ptr)++,
                     &g, sizeof(g),
                     0);
    if (n < 0) return -1;
    return write_all(daemon_fd, buf, (size_t)n);
}
