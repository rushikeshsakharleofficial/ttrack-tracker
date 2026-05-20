#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <errno.h>
#include <poll.h>
#include <signal.h>
#include <sys/ioctl.h>
#include <sys/wait.h>
#include <fcntl.h>
#include <time.h>

#include "trackterm_proto.h"
#include "trackterm_ttyrec.h"
#include "trackterm_log.h"
#include "trackterm_compat.h"
#include "ringbuf.h"

extern volatile sig_atomic_t trackterm_winch_pending;
extern volatile sig_atomic_t trackterm_child_exited;
extern volatile sig_atomic_t trackterm_got_sigterm;
extern int trackterm_child_status;
int trackterm_reap_child(pid_t pid);

/* Declared in session.c */
int trackterm_session_send_out(int daemon_fd, uint64_t *seq_ptr,
                         const uint8_t *data, uint32_t len);
int trackterm_session_send_close(int daemon_fd, uint64_t *seq_ptr, int exit_code);
int trackterm_session_send_resize(int daemon_fd, uint64_t *seq_ptr,
                            uint16_t rows, uint16_t cols);

/* Declared in pty.c */
int trackterm_pty_get_winsize(int fd, struct winsize *ws);
int trackterm_pty_set_winsize(int master_fd, const struct winsize *ws);

static ssize_t write_all_fd(int fd, const void *buf, size_t n)
{
    const uint8_t *p = buf;
    while (n > 0) {
        ssize_t r = write(fd, p, n);
        if (r < 0) {
            if (errno == EINTR) continue;
            if (errno == EAGAIN || errno == EWOULDBLOCK) return (ssize_t)(n); /* partial */
            return -1;
        }
        p += r;
        n -= (size_t)r;
    }
    return 0;
}

#define IOBUF_SIZE  65536

typedef struct {
    int            stdin_fd;
    int            stdout_fd;
    int            master_fd;
    int            daemon_fd;      /* -1 = no daemon (M1 local mode) */
    int            local_fd;       /* local ttyrec file, -1 if daemon mode */
    int            local_evt_fd;   /* local events.jsonl, -1 if daemon mode */
    pid_t          child_pid;
    char           sid[37];
    char           parent_sid[37];
    uint32_t       loginuid;
    uint64_t       seq;
    uint64_t       bytes_dropped;
    int            fail_closed;
    struct ringbuf *daemon_buf;
    struct timespec session_start;
} shim_ctx_t;

static double elapsed_sec(const struct timespec *start)
{
    struct timespec now;
    clock_gettime(CLOCK_REALTIME, &now);
    return (double)(now.tv_sec - start->tv_sec) +
           (double)(now.tv_nsec - start->tv_nsec) / 1e9;
}

static void flush_daemon_buf(shim_ctx_t *ctx)
{
    if (!ctx->daemon_buf || ctx->daemon_fd < 0) return;

    uint8_t tmp[4096];
    size_t avail;
    while ((avail = ringbuf_used(ctx->daemon_buf)) > 0) {
        size_t n = ringbuf_peek(ctx->daemon_buf, tmp,
                                avail < sizeof(tmp) ? avail : sizeof(tmp));
        ssize_t r = write(ctx->daemon_fd, tmp, n);
        if (r <= 0) break;
        ringbuf_consume(ctx->daemon_buf, (size_t)r);
    }
}

static void send_output(shim_ctx_t *ctx, const uint8_t *data, size_t len)
{
    while (len > 0) {
        uint32_t chunk = (uint32_t)(len > 65536 ? 65536 : len);

        /* Local file mode (M1 / TRACKTERM_REC_NO_DAEMON) */
        if (ctx->local_fd >= 0) {
            ttyrec_write_frame(ctx->local_fd, data, chunk);
        }

        /* Daemon mode */
        if (ctx->daemon_fd >= 0) {
            trackterm_session_send_out(ctx->daemon_fd, &ctx->seq, data, chunk);
        }

        data += chunk;
        len  -= chunk;
    }
}

static void handle_winch(shim_ctx_t *ctx)
{
    struct winsize ws;
    if (trackterm_pty_get_winsize(ctx->stdin_fd, &ws) < 0) return;
    trackterm_pty_set_winsize(ctx->master_fd, &ws);

    double t = elapsed_sec(&ctx->session_start);
    char body[128];
    snprintf(body, sizeof(body),
             "\"type\":\"resize\",\"rows\":%u,\"cols\":%u",
             ws.ws_row, ws.ws_col);
    if (ctx->local_evt_fd >= 0)
        ttyrec_write_event(ctx->local_evt_fd, t, body);

    if (ctx->daemon_fd >= 0)
        trackterm_session_send_resize(ctx->daemon_fd, &ctx->seq, ws.ws_row, ws.ws_col);
}

int trackterm_shim_loop_run(shim_ctx_t *ctx)
{
    uint8_t buf[IOBUF_SIZE];
    struct pollfd fds[3];
    int nfds = 2;

    fds[0].fd     = ctx->stdin_fd;
    fds[0].events = POLLIN;
    fds[1].fd     = ctx->master_fd;
    fds[1].events = POLLIN;

    if (ctx->daemon_fd >= 0) {
        fds[2].fd     = ctx->daemon_fd;
        fds[2].events = 0; /* monitor for POLLHUP only */
        nfds = 3;
    }

    for (;;) {
        if (trackterm_winch_pending) {
            trackterm_winch_pending = 0;
            handle_winch(ctx);
        }

        if (trackterm_child_exited) {
            trackterm_child_exited = 0;
            trackterm_reap_child(ctx->child_pid);
            TRACKTERM_LOG_INFO("loop: child exited status=%d", trackterm_child_status);
            /* Drain remaining master output */
            for (;;) {
                ssize_t r = read(ctx->master_fd, buf, sizeof(buf));
                if (r <= 0) break;
                write_all_fd(ctx->stdout_fd, buf, (size_t)r);
                send_output(ctx, buf, (size_t)r);
            }
            break;
        }

        if (trackterm_got_sigterm) {
            TRACKTERM_LOG_INFO("loop: got SIGHUP/SIGTERM");
            /* Propagate termination to child before exiting loop */
            kill(ctx->child_pid, SIGHUP);
            break;
        }

        int r = poll(fds, (nfds_t)nfds, 100 /* ms — for signal check */);
        if (r < 0 && errno != EINTR) break;
        if (r == 0) continue;

        /* stdin → master */
        if (fds[0].revents & POLLIN) {
            ssize_t n = read(ctx->stdin_fd, buf, sizeof(buf));
            if (n > 0)
                write_all_fd(ctx->master_fd, buf, (size_t)n);
            else if (n == 0 || (n < 0 && errno != EINTR && errno != EAGAIN)) {
                /* stdin EOF: close write end of master */
                /* Sending EOF to PTY: write Ctrl-D */
                write(ctx->master_fd, "\x04", 1);
            }
        }

        /* master → stdout + record */
        if (fds[1].revents & POLLIN) {
            ssize_t n = read(ctx->master_fd, buf, sizeof(buf));
            if (n > 0) {
                write_all_fd(ctx->stdout_fd, buf, (size_t)n);
                send_output(ctx, buf, (size_t)n);
            } else if (n == 0 || (n < 0 && errno == EIO)) {
                TRACKTERM_LOG_INFO("loop: master EIO/EOF (child closed)");
                /* Child exited, master closed */
                break;
            }
        }

        if (fds[1].revents & (POLLHUP | POLLERR)) {
            TRACKTERM_LOG_INFO("loop: master POLLHUP/POLLERR");
            break;
        }

        /* Daemon disconnect */
        if (nfds == 3 && (fds[2].revents & (POLLHUP | POLLERR))) {
            TRACKTERM_LOG_WARN("daemon connection lost — recording paused");
            close(ctx->daemon_fd);
            ctx->daemon_fd = -1;
            nfds = 2;
        }
    }

    /* Final reap: if we broke out before SIGCHLD was processed.
     * If loop exited via SIGHUP/SIGTERM while child is still alive,
     * kill the child first so we don't block forever. */
    if (trackterm_child_status == 0) {
        int wstatus;
        pid_t r = waitpid(ctx->child_pid, &wstatus, WNOHANG);
        if (r <= 0) {
            /* Child still running — send SIGHUP then wait up to 2s */
            kill(ctx->child_pid, SIGHUP);
            int retries = 20;
            while (retries-- > 0) {
                struct timespec ts = { .tv_sec = 0, .tv_nsec = 100000000L };
                nanosleep(&ts, NULL);
                r = waitpid(ctx->child_pid, &wstatus, WNOHANG);
                if (r > 0) break;
            }
            if (r <= 0) {
                kill(ctx->child_pid, SIGKILL);
                waitpid(ctx->child_pid, &wstatus, 0);
                r = ctx->child_pid;
                wstatus = 0;
            }
        }
        if (r > 0) {
            if (WIFEXITED(wstatus))
                trackterm_child_status = WEXITSTATUS(wstatus);
            else if (WIFSIGNALED(wstatus))
                trackterm_child_status = 128 + WTERMSIG(wstatus);
        }
    }

    return trackterm_child_status;
}
