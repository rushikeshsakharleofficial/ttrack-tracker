#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <fcntl.h>
#include <errno.h>
#include <signal.h>
#include <sys/ioctl.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <termios.h>
#include <time.h>
#include <pwd.h>

#include "trackterm_proto.h"
#include "trackterm_ttyrec.h"
#include "trackterm_uuid.h"
#include "trackterm_paths.h"
#include "trackterm_log.h"
#include "trackterm_compat.h"
#include "ringbuf.h"

/* Declarations from shim units */
int  trackterm_pty_open(int *amaster, int *aslave, char **slave_name,
                  const struct winsize *winp);
int  trackterm_pty_child_setup(int slave_fd);
int  trackterm_pty_set_raw(int fd, struct termios *saved);
int  trackterm_pty_restore(int fd, const struct termios *saved);
int  trackterm_pty_get_winsize(int fd, struct winsize *ws);
int  trackterm_signals_install(void);
char *trackterm_resolve_shell(void);
char **trackterm_build_shell_argv(const char *shell, char *const *explicit_argv, int login);
int  trackterm_already_recording(void);
void trackterm_env_stamp_child(const char *sid, const char *parent_sid,
                         uint32_t loginuid, const char *real_shell);
const char *trackterm_env_get_sid(void);
const char *trackterm_env_get_parent_sid(void);
uint32_t trackterm_session_read_loginuid(void);
int  trackterm_session_connect_daemon(int fail_closed);
int  trackterm_session_send_hello(int daemon_fd,
                            const char *sid, const char *parent_sid,
                            uint32_t loginuid, const char *service,
                            const char *tty, uint16_t rows, uint16_t cols,
                            const char *rhost);
int trackterm_session_send_close(int daemon_fd, uint64_t *seq_ptr, int exit_code);

typedef struct {
    int            stdin_fd;
    int            stdout_fd;
    int            master_fd;
    int            daemon_fd;
    int            local_fd;
    int            local_evt_fd;
    pid_t          child_pid;
    char           sid[37];
    char           parent_sid[37];
    char           service[64];
    char           tty[64];
    char           rhost[64];
    uint16_t       rows;
    uint16_t       cols;
    uint32_t       loginuid;
    uint64_t       seq;
    uint64_t       bytes_dropped;
    int            fail_closed;
    time_t         disconnect_ts;
    struct ringbuf *daemon_buf;
    struct timespec session_start;
} shim_ctx_t;

int trackterm_shim_loop_run(shim_ctx_t *ctx);

static struct termios g_saved_termios;
static int g_stdin_is_raw = 0;
static int g_tty_fd = STDIN_FILENO;

static double elapsed_sec(const struct timespec *start)
{
    struct timespec now;
    clock_gettime(CLOCK_REALTIME, &now);
    return (double)(now.tv_sec - start->tv_sec) +
           (double)(now.tv_nsec - start->tv_nsec) / 1e9;
}

static void restore_termios_atexit(void)
{
    if (g_stdin_is_raw)
        tcsetattr(g_tty_fd, TCSANOW, &g_saved_termios);
}

static void restore_termios_on_signal(int sig)
{
    if (g_stdin_is_raw)
        tcsetattr(g_tty_fd, TCSANOW, &g_saved_termios);
    signal(sig, SIG_DFL);
    raise(sig);
}

static int open_local_outfiles(const char *sid,
                               int *ttyrec_fd_out, int *evt_fd_out)
{
    const char *outdir = getenv("TRACKTERM_REC_OUTDIR");
    char path[512];
    int fd, efd;

    if (!outdir) outdir = "/tmp/trackterm-rec";

    if (mkdir(outdir, 0700) < 0 && errno != EEXIST) {
        TRACKTERM_LOG_ERR("mkdir %s: %s", outdir, strerror(errno));
        return -1;
    }

    snprintf(path, sizeof(path), "%s/%s.ttyrec", outdir, sid);
    fd = open(path, O_WRONLY | O_CREAT | O_TRUNC | O_CLOEXEC, 0600);
    if (fd < 0) {
        TRACKTERM_LOG_ERR("open %s: %s", path, strerror(errno));
        return -1;
    }

    snprintf(path, sizeof(path), "%s/%s.events.jsonl", outdir, sid);
    efd = open(path, O_WRONLY | O_CREAT | O_TRUNC | O_CLOEXEC, 0600);
    if (efd < 0) {
        close(fd);
        return -1;
    }

    *ttyrec_fd_out = fd;
    *evt_fd_out    = efd;
    return 0;
}

int main(int argc, char *argv[])
{
    shim_ctx_t ctx;
    int master, slave;
    struct winsize ws;
    char *slave_name = NULL;
    char *shell = NULL;
    char **shell_argv = NULL;
    pid_t child;
    int exit_code = 1;
    int no_daemon;
    int fail_closed = 0;

    /* Always log to syslog — stderr may be unreachable from profile.d */
    trackterm_log_init("trackterm-rec", 0, LOG_INFO);

    memset(&ctx, 0, sizeof(ctx));
    ctx.daemon_fd    = -1;
    ctx.local_fd     = -1;
    ctx.local_evt_fd = -1;

    TRACKTERM_LOG_INFO("trackterm-rec start: pid=%d stdin_tty=%d stdout_tty=%d",
                 (int)getpid(), isatty(STDIN_FILENO), isatty(STDOUT_FILENO));

    /* bash redirects fd 1 internally during profile.d sourcing, so STDOUT_FILENO
     * is not the terminal.  Open /dev/tty to get the real controlling terminal
     * regardless of fd redirections.  Fall back to STDIN_FILENO (isatty(0)=YES). */
    {
        int tty_fd = open("/dev/tty", O_RDWR | O_CLOEXEC);
        if (tty_fd >= 0) {
            ctx.stdin_fd  = tty_fd;
            ctx.stdout_fd = tty_fd;
            TRACKTERM_LOG_INFO("trackterm-rec: /dev/tty fd=%d", tty_fd);
        } else {
            ctx.stdin_fd  = STDIN_FILENO;
            ctx.stdout_fd = STDIN_FILENO;
            TRACKTERM_LOG_WARN("trackterm-rec: open /dev/tty failed: %s, using fd0", strerror(errno));
        }
    }

    /* Re-entrancy guard */
    if (trackterm_already_recording()) {
        TRACKTERM_LOG_INFO("already-recording: exec shell directly");
        shell = trackterm_resolve_shell();
        shell_argv = trackterm_build_shell_argv(shell, argc > 1 ? argv + 1 : NULL, 0);
        execvp(shell, shell_argv);
        _exit(127);
    }

    /* Non-interactive bypass: scp / sftp / ssh host cmd
     * TRACKTERM_REC_FORCE_PTY=1 disables this check (testing only). */
    {
        const char *force = getenv("TRACKTERM_REC_FORCE_PTY");
        int force_pty = (force && force[0] == '1');
        /* Only check stdin — stdout may not be a tty during profile.d sourcing
         * (bash redirects it internally). Checking stdout causes SIGPIPE on
         * the exec'd shell when the internal bash pipe's read end is gone. */
        if (!force_pty && !isatty(STDIN_FILENO)) {
            shell = trackterm_resolve_shell();
            shell_argv = trackterm_build_shell_argv(shell, argc > 1 ? argv + 1 : NULL, 0);
            execvp(shell, shell_argv);
            _exit(127);
        }
    }

    /* Build SID — inherit from PAM env if available */
    {
        const char *env_sid    = trackterm_env_get_sid();
        const char *env_parent = trackterm_env_get_parent_sid();

        if (env_sid && env_sid[0])
            strncpy(ctx.sid, env_sid, 36);
        else if (trackterm_uuid_generate(ctx.sid) < 0) {
            TRACKTERM_LOG_ERR("uuid generate failed");
            _exit(1);
        }

        if (env_parent && env_parent[0])
            strncpy(ctx.parent_sid, env_parent, 36);
    }

    ctx.loginuid = trackterm_session_read_loginuid();

    g_tty_fd = ctx.stdin_fd;

    /* Get terminal size */
    if (trackterm_pty_get_winsize(ctx.stdin_fd, &ws) < 0) {
        ws.ws_row = 24; ws.ws_col = 80;
        ws.ws_xpixel = 0; ws.ws_ypixel = 0;
    }

    no_daemon = (getenv("TRACKTERM_REC_NO_DAEMON") != NULL);
    {
        const char *fc = getenv("TRACKTERM_REC_FAIL_CLOSED");
        if (fc && fc[0] == '1') fail_closed = 1;
    }

    if (!no_daemon) {
        ctx.daemon_fd = trackterm_session_connect_daemon(fail_closed);
        if (ctx.daemon_fd < 0 && fail_closed) _exit(1);
    }
    TRACKTERM_LOG_INFO("trackterm-rec: daemon_fd=%d", ctx.daemon_fd);

    if (ctx.daemon_fd < 0) {
        if (open_local_outfiles(ctx.sid, &ctx.local_fd, &ctx.local_evt_fd) < 0) {
            if (fail_closed) _exit(1);
            /* fail-open: continue without recording */
        }
    }
    TRACKTERM_LOG_INFO("trackterm-rec: local_fd=%d evt_fd=%d", ctx.local_fd, ctx.local_evt_fd);

    /* Resolve shell */
    shell = (argc > 1) ? strdup(argv[1]) : trackterm_resolve_shell();
    TRACKTERM_LOG_INFO("trackterm-rec: shell=%s", shell ? shell : "(null)");

    /* Allocate PTY */
    if (trackterm_pty_open(&master, &slave, &slave_name, &ws) < 0) {
        TRACKTERM_LOG_ERR("openpty: %s", strerror(errno));
        _exit(1);
    }
    TRACKTERM_LOG_INFO("trackterm-rec: pty master=%d slave=%d name=%s", master, slave, slave_name ? slave_name : "?");
    ctx.master_fd = master;

    trackterm_signals_install();

    /* Put user terminal into raw mode */
    if (trackterm_pty_set_raw(ctx.stdin_fd, &g_saved_termios) == 0) {
        g_stdin_is_raw = 1;
        atexit(restore_termios_atexit);
        signal(SIGINT, restore_termios_on_signal);
    }

    clock_gettime(CLOCK_REALTIME, &ctx.session_start);

    /* Write start event */
    if (ctx.local_evt_fd >= 0) {
        char body[384];
        snprintf(body, sizeof(body),
                 "\"type\":\"start\",\"rows\":%u,\"cols\":%u,"
                 "\"term\":\"%s\",\"tty\":\"%s\","
                 "\"sid\":\"%s\",\"parent_sid\":\"%s\"",
                 ws.ws_row, ws.ws_col,
                 getenv("TERM") ? getenv("TERM") : "xterm",
                 slave_name ? slave_name : "",
                 ctx.sid, ctx.parent_sid);
        ttyrec_write_event(ctx.local_evt_fd, 0.0, body);
    }

    /* Cache session metadata for reconnect */
    strncpy(ctx.service, getenv("TRACKTERM_REC_SERVICE") ? getenv("TRACKTERM_REC_SERVICE") :
                         (getenv("PAM_SERVICE") ? getenv("PAM_SERVICE") : "unknown"),
            sizeof(ctx.service) - 1);
    strncpy(ctx.tty,   slave_name ? slave_name : "",         sizeof(ctx.tty)   - 1);
    strncpy(ctx.rhost, getenv("SSH_CLIENT") ? getenv("SSH_CLIENT") : "",
            sizeof(ctx.rhost) - 1);
    ctx.rows = ws.ws_row;
    ctx.cols = ws.ws_col;

    /* Send hello to daemon */
    if (ctx.daemon_fd >= 0) {
        trackterm_session_send_hello(ctx.daemon_fd,
                               ctx.sid, ctx.parent_sid,
                               ctx.loginuid,
                               ctx.service, ctx.tty,
                               ctx.rows, ctx.cols,
                               ctx.rhost);
    }

    TRACKTERM_LOG_INFO("trackterm-rec: about to fork");
    /* Fork */
    child = fork();
    if (child < 0) {
        TRACKTERM_LOG_ERR("fork: %s", strerror(errno));
        _exit(1);
    }

    if (child == 0) {
        close(master);
        if (trackterm_pty_child_setup(slave) < 0) _exit(1);

        trackterm_env_stamp_child(ctx.sid, ctx.parent_sid, ctx.loginuid, shell);

        shell_argv = trackterm_build_shell_argv(shell, argc > 1 ? argv + 1 : NULL, 0);
        if (shell_argv)
            execvp(shell_argv[0], shell_argv);
        perror("execvp");
        _exit(127);
    }

    /* Parent */
    TRACKTERM_LOG_INFO("trackterm-rec: parent, child_pid=%d", (int)child);
    close(slave);
    ctx.child_pid = child;

    if (ctx.daemon_fd >= 0)
        ctx.daemon_buf = ringbuf_alloc(8 * 1024 * 1024);

    TRACKTERM_LOG_INFO("trackterm-rec: entering loop");
    exit_code = trackterm_shim_loop_run(&ctx);
    TRACKTERM_LOG_INFO("trackterm-rec: loop exited exit_code=%d", exit_code);

    /* Send close frame */
    if (ctx.daemon_fd >= 0)
        trackterm_session_send_close(ctx.daemon_fd, &ctx.seq, exit_code);

    /* Write end event */
    if (ctx.local_evt_fd >= 0) {
        double t = elapsed_sec(&ctx.session_start);
        char body[64];
        snprintf(body, sizeof(body), "\"type\":\"end\",\"exit\":%d", exit_code);
        ttyrec_write_event(ctx.local_evt_fd, t, body);
        close(ctx.local_evt_fd);
    }

    if (ctx.local_fd   >= 0) close(ctx.local_fd);
    if (ctx.daemon_fd  >= 0) close(ctx.daemon_fd);
    if (ctx.daemon_buf)      ringbuf_free(ctx.daemon_buf);
    if (ctx.master_fd  >= 0) close(ctx.master_fd);

    if (g_stdin_is_raw)
        trackterm_pty_restore(ctx.stdin_fd, &g_saved_termios);

    free(shell);
    free(slave_name);

    return exit_code;
}
