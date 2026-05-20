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

#include "pmp_proto.h"
#include "pmp_ttyrec.h"
#include "pmp_uuid.h"
#include "pmp_paths.h"
#include "pmp_log.h"
#include "pmp_compat.h"
#include "ringbuf.h"

/* Declarations from shim units */
int  pmp_pty_open(int *amaster, int *aslave, char **slave_name,
                  const struct winsize *winp);
int  pmp_pty_child_setup(int slave_fd);
int  pmp_pty_set_raw(int fd, struct termios *saved);
int  pmp_pty_restore(int fd, const struct termios *saved);
int  pmp_pty_get_winsize(int fd, struct winsize *ws);
int  pmp_signals_install(void);
char *pmp_resolve_shell(void);
char **pmp_build_shell_argv(const char *shell, char *const *explicit_argv, int login);
int  pmp_already_recording(void);
void pmp_env_stamp_child(const char *sid, const char *parent_sid,
                         uint32_t loginuid, const char *real_shell);
const char *pmp_env_get_sid(void);
const char *pmp_env_get_parent_sid(void);
uint32_t pmp_session_read_loginuid(void);
int  pmp_session_connect_daemon(int fail_closed);
int  pmp_session_send_hello(int daemon_fd,
                            const char *sid, const char *parent_sid,
                            uint32_t loginuid, const char *service,
                            const char *tty, uint16_t rows, uint16_t cols,
                            const char *rhost);
int pmp_session_send_close(int daemon_fd, uint64_t *seq_ptr, int exit_code);

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
    uint32_t       loginuid;
    uint64_t       seq;
    uint64_t       bytes_dropped;
    int            fail_closed;
    struct ringbuf *daemon_buf;
    struct timespec session_start;
} shim_ctx_t;

int pmp_shim_loop_run(shim_ctx_t *ctx);

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
    const char *outdir = getenv("PMP_REC_OUTDIR");
    char path[512];
    int fd, efd;

    if (!outdir) outdir = "/tmp/pmp-rec";

    if (mkdir(outdir, 0700) < 0 && errno != EEXIST) {
        PMP_LOG_ERR("mkdir %s: %s", outdir, strerror(errno));
        return -1;
    }

    snprintf(path, sizeof(path), "%s/%s.ttyrec", outdir, sid);
    fd = open(path, O_WRONLY | O_CREAT | O_TRUNC | O_CLOEXEC, 0600);
    if (fd < 0) {
        PMP_LOG_ERR("open %s: %s", path, strerror(errno));
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
    pmp_log_init("pmp-rec", 0, LOG_INFO);

    memset(&ctx, 0, sizeof(ctx));
    ctx.daemon_fd    = -1;
    ctx.local_fd     = -1;
    ctx.local_evt_fd = -1;

    PMP_LOG_INFO("pmp-rec start: pid=%d stdin_tty=%d stdout_tty=%d",
                 (int)getpid(), isatty(STDIN_FILENO), isatty(STDOUT_FILENO));

    /* bash redirects fd 1 internally during profile.d sourcing, so STDOUT_FILENO
     * is not the terminal.  Open /dev/tty to get the real controlling terminal
     * regardless of fd redirections.  Fall back to STDIN_FILENO (isatty(0)=YES). */
    {
        int tty_fd = open("/dev/tty", O_RDWR | O_CLOEXEC);
        if (tty_fd >= 0) {
            ctx.stdin_fd  = tty_fd;
            ctx.stdout_fd = tty_fd;
            PMP_LOG_INFO("pmp-rec: /dev/tty fd=%d", tty_fd);
        } else {
            ctx.stdin_fd  = STDIN_FILENO;
            ctx.stdout_fd = STDIN_FILENO;
            PMP_LOG_WARN("pmp-rec: open /dev/tty failed: %s, using fd0", strerror(errno));
        }
    }

    /* Re-entrancy guard */
    if (pmp_already_recording()) {
        PMP_LOG_INFO("already-recording: exec shell directly");
        shell = pmp_resolve_shell();
        shell_argv = pmp_build_shell_argv(shell, argc > 1 ? argv + 1 : NULL, 0);
        execvp(shell, shell_argv);
        _exit(127);
    }

    /* Non-interactive bypass: scp / sftp / ssh host cmd
     * PMP_REC_FORCE_PTY=1 disables this check (testing only). */
    {
        const char *force = getenv("PMP_REC_FORCE_PTY");
        int force_pty = (force && force[0] == '1');
        /* Only check stdin — stdout may not be a tty during profile.d sourcing
         * (bash redirects it internally). Checking stdout causes SIGPIPE on
         * the exec'd shell when the internal bash pipe's read end is gone. */
        if (!force_pty && !isatty(STDIN_FILENO)) {
            shell = pmp_resolve_shell();
            shell_argv = pmp_build_shell_argv(shell, argc > 1 ? argv + 1 : NULL, 0);
            execvp(shell, shell_argv);
            _exit(127);
        }
    }

    /* Build SID — inherit from PAM env if available */
    {
        const char *env_sid    = pmp_env_get_sid();
        const char *env_parent = pmp_env_get_parent_sid();

        if (env_sid && env_sid[0])
            strncpy(ctx.sid, env_sid, 36);
        else if (pmp_uuid_generate(ctx.sid) < 0) {
            PMP_LOG_ERR("uuid generate failed");
            _exit(1);
        }

        if (env_parent && env_parent[0])
            strncpy(ctx.parent_sid, env_parent, 36);
    }

    ctx.loginuid = pmp_session_read_loginuid();

    g_tty_fd = ctx.stdin_fd;

    /* Get terminal size */
    if (pmp_pty_get_winsize(ctx.stdin_fd, &ws) < 0) {
        ws.ws_row = 24; ws.ws_col = 80;
        ws.ws_xpixel = 0; ws.ws_ypixel = 0;
    }

    no_daemon = (getenv("PMP_REC_NO_DAEMON") != NULL);
    {
        const char *fc = getenv("PMP_REC_FAIL_CLOSED");
        if (fc && fc[0] == '1') fail_closed = 1;
    }

    if (!no_daemon) {
        ctx.daemon_fd = pmp_session_connect_daemon(fail_closed);
        if (ctx.daemon_fd < 0 && fail_closed) _exit(1);
    }
    PMP_LOG_INFO("pmp-rec: daemon_fd=%d", ctx.daemon_fd);

    if (ctx.daemon_fd < 0) {
        if (open_local_outfiles(ctx.sid, &ctx.local_fd, &ctx.local_evt_fd) < 0) {
            if (fail_closed) _exit(1);
            /* fail-open: continue without recording */
        }
    }
    PMP_LOG_INFO("pmp-rec: local_fd=%d evt_fd=%d", ctx.local_fd, ctx.local_evt_fd);

    /* Resolve shell */
    shell = (argc > 1) ? strdup(argv[1]) : pmp_resolve_shell();
    PMP_LOG_INFO("pmp-rec: shell=%s", shell ? shell : "(null)");

    /* Allocate PTY */
    if (pmp_pty_open(&master, &slave, &slave_name, &ws) < 0) {
        PMP_LOG_ERR("openpty: %s", strerror(errno));
        _exit(1);
    }
    PMP_LOG_INFO("pmp-rec: pty master=%d slave=%d name=%s", master, slave, slave_name ? slave_name : "?");
    ctx.master_fd = master;

    pmp_signals_install();

    /* Put user terminal into raw mode */
    if (pmp_pty_set_raw(ctx.stdin_fd, &g_saved_termios) == 0) {
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

    /* Send hello to daemon */
    if (ctx.daemon_fd >= 0) {
        pmp_session_send_hello(ctx.daemon_fd,
                               ctx.sid, ctx.parent_sid,
                               ctx.loginuid,
                               getenv("PMP_REC_SERVICE") ? getenv("PMP_REC_SERVICE") :
                               getenv("PAM_SERVICE")     ? getenv("PAM_SERVICE")     : "unknown",
                               slave_name, ws.ws_row, ws.ws_col,
                               getenv("SSH_CLIENT"));
    }

    PMP_LOG_INFO("pmp-rec: about to fork");
    /* Fork */
    child = fork();
    if (child < 0) {
        PMP_LOG_ERR("fork: %s", strerror(errno));
        _exit(1);
    }

    if (child == 0) {
        close(master);
        if (pmp_pty_child_setup(slave) < 0) _exit(1);

        pmp_env_stamp_child(ctx.sid, ctx.parent_sid, ctx.loginuid, shell);

        shell_argv = pmp_build_shell_argv(shell, argc > 1 ? argv + 1 : NULL, 0);
        if (shell_argv)
            execvp(shell_argv[0], shell_argv);
        perror("execvp");
        _exit(127);
    }

    /* Parent */
    PMP_LOG_INFO("pmp-rec: parent, child_pid=%d", (int)child);
    close(slave);
    ctx.child_pid = child;

    if (ctx.daemon_fd >= 0)
        ctx.daemon_buf = ringbuf_alloc(8 * 1024 * 1024);

    PMP_LOG_INFO("pmp-rec: entering loop");
    exit_code = pmp_shim_loop_run(&ctx);
    PMP_LOG_INFO("pmp-rec: loop exited exit_code=%d", exit_code);

    /* Send close frame */
    if (ctx.daemon_fd >= 0)
        pmp_session_send_close(ctx.daemon_fd, &ctx.seq, exit_code);

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
        pmp_pty_restore(ctx.stdin_fd, &g_saved_termios);

    free(shell);
    free(slave_name);

    return exit_code;
}
