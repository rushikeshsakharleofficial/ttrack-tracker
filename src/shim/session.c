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

#include "pmp_proto.h"
#include "pmp_paths.h"
#include "pmp_uuid.h"
#include "pmp_log.h"
#include "pmp_compat.h"

/* Defined in loop.c / main.c for the shim context */
typedef struct pmp_shim_ctx pmp_shim_ctx_t;
struct pmp_shim_ctx;

uint32_t pmp_session_read_loginuid(void)
{
    char buf[32];
    int fd;
    ssize_t n;
    uint32_t val;

    fd = open(PMP_LOGINUID_PATH, O_RDONLY | O_CLOEXEC);
    if (fd < 0) return PMP_LOGINUID_UNSET;

    n = read(fd, buf, sizeof(buf) - 1);
    close(fd);

    if (n <= 0) return PMP_LOGINUID_UNSET;
    buf[n] = '\0';

    val = (uint32_t)strtoul(buf, NULL, 10);
    return val;
}

int pmp_session_connect_daemon(int fail_closed)
{
    struct sockaddr_un addr;
    int fd, i;
    int tries = fail_closed ? 5 : 3;

    for (i = 0; i < tries; i++) {
        fd = socket(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0);
        if (fd < 0) return -1;

        memset(&addr, 0, sizeof(addr));
        addr.sun_family = AF_UNIX;
        strncpy(addr.sun_path, PMP_SOCK_PATH, sizeof(addr.sun_path) - 1);

        if (connect(fd, (struct sockaddr *)&addr, sizeof(addr)) == 0)
            return fd;

        close(fd);

        if (i < tries - 1) {
            struct timespec ts = { .tv_sec = 0, .tv_nsec = 200000000L }; /* 200ms */
            nanosleep(&ts, NULL);
        }
    }

    if (fail_closed)
        PMP_LOG_ERR("pmp-recd unreachable at %s — session denied (fail_closed)", PMP_SOCK_PATH);
    else
        PMP_LOG_WARN("pmp-recd unreachable at %s — recording disabled", PMP_SOCK_PATH);

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

int pmp_session_send_hello(int daemon_fd,
                           const char *sid, const char *parent_sid,
                           uint32_t loginuid, const char *service,
                           const char *tty, uint16_t rows, uint16_t cols,
                           const char *rhost)
{
    struct pmp_hello hello;
    uint8_t buf[sizeof(struct pmp_frame_hdr) + sizeof(hello)];
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
                     PMP_F_HELLO,
                     pmp_clock_ns(), 0,
                     &hello, sizeof(hello),
                     1 /* include_magic */);
    if (n < 0) return -1;

    return (int)write_all(daemon_fd, buf, (size_t)n);
}

int pmp_session_send_close(int daemon_fd, uint64_t *seq_ptr, int exit_code)
{
    struct pmp_close cl;
    uint8_t buf[sizeof(struct pmp_frame_hdr) + sizeof(cl)];
    ssize_t n;

    cl.exit_status = (int32_t)exit_code;

    n = frame_encode(buf, sizeof(buf),
                     PMP_F_CLOSE,
                     pmp_clock_ns(), (*seq_ptr)++,
                     &cl, sizeof(cl),
                     0);
    if (n < 0) return -1;

    return (int)write_all(daemon_fd, buf, (size_t)n);
}

int pmp_session_send_resize(int daemon_fd, uint64_t *seq_ptr,
                            uint16_t rows, uint16_t cols)
{
    struct pmp_resize rs;
    uint8_t buf[sizeof(struct pmp_frame_hdr) + sizeof(rs)];
    ssize_t n;

    rs.rows = rows; rs.cols = cols; rs.xpixel = 0; rs.ypixel = 0;

    n = frame_encode(buf, sizeof(buf),
                     PMP_F_RESIZE,
                     pmp_clock_ns(), (*seq_ptr)++,
                     &rs, sizeof(rs),
                     0);
    if (n < 0) return -1;

    return write_all(daemon_fd, buf, (size_t)n);
}

int pmp_session_send_out(int daemon_fd, uint64_t *seq_ptr,
                         const uint8_t *data, uint32_t len)
{
    uint8_t buf[sizeof(struct pmp_frame_hdr)];
    uint8_t hdr_and_data[sizeof(struct pmp_frame_hdr) + 65536];
    ssize_t n;

    if (len > 65536) len = 65536;

    n = frame_encode(hdr_and_data, sizeof(hdr_and_data),
                     PMP_F_OUT,
                     pmp_clock_ns(), (*seq_ptr)++,
                     data, len,
                     0);
    if (n < 0) return -1;

    (void)buf;
    return write_all(daemon_fd, hdr_and_data, (size_t)n);
}
