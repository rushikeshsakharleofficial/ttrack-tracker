#define _GNU_SOURCE
#include <signal.h>
#include <sys/wait.h>
#include <errno.h>
#include <string.h>
#include "trackterm_log.h"

volatile sig_atomic_t trackterm_winch_pending = 0;
volatile sig_atomic_t trackterm_child_exited  = 0;
volatile sig_atomic_t trackterm_got_sigterm   = 0;
int trackterm_child_status = 0;

static void sigwinch_handler(int sig)
{
    (void)sig;
    trackterm_winch_pending = 1;
}

static void sigchld_handler(int sig)
{
    (void)sig;
    trackterm_child_exited = 1;
}

static void sigterm_handler(int sig)
{
    (void)sig;
    trackterm_got_sigterm = 1;
}

int trackterm_signals_install(void)
{
    struct sigaction sa;

    sigemptyset(&sa.sa_mask);
    sa.sa_flags = SA_RESTART;

    sa.sa_handler = sigwinch_handler;
    if (sigaction(SIGWINCH, &sa, NULL) < 0) return -1;

    sa.sa_flags = SA_RESTART | SA_NOCLDSTOP;
    sa.sa_handler = sigchld_handler;
    if (sigaction(SIGCHLD, &sa, NULL) < 0) return -1;

    sa.sa_flags = SA_RESTART;
    sa.sa_handler = sigterm_handler;
    if (sigaction(SIGTERM, &sa, NULL) < 0) return -1;
    if (sigaction(SIGHUP,  &sa, NULL) < 0) return -1;

    /* Ignore SIGPIPE — handle EPIPE from write() directly */
    signal(SIGPIPE, SIG_IGN);

    return 0;
}

/* Reap child non-blockingly. Returns exit code or -1 if not ready. */
int trackterm_reap_child(pid_t pid)
{
    int status;
    pid_t r = waitpid(pid, &status, WNOHANG);
    if (r <= 0) return -1;

    if (WIFEXITED(status))
        trackterm_child_status = WEXITSTATUS(status);
    else if (WIFSIGNALED(status))
        trackterm_child_status = 128 + WTERMSIG(status);
    else
        trackterm_child_status = 1;

    return trackterm_child_status;
}
