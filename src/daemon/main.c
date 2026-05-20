#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <signal.h>
#include <errno.h>
#include <sys/epoll.h>
#include <sys/stat.h>
#include <fcntl.h>
#include <time.h>

#ifdef HAVE_SYSTEMD
#include <systemd/sd-daemon.h>
#else
#define sd_notify(u, s) (0)
#define sd_notifyf(u, f, ...) (0)
#endif

#include "pmp_proto.h"
#include "pmp_log.h"
#include "pmp_paths.h"

/* Forward declarations */
int  pmp_server_bind(const char *sockpath, mode_t mode);
int  pmp_server_accept(int listen_fd, const char *storage_dir, int epoll_fd);
int  pmp_server_handle_client(void *client, int epoll_fd);
void pmp_server_drop_client(void *client, int epoll_fd);
int  pmp_config_load(const char *path);

typedef struct pmp_daemon_config {
    char    storage_dir[256];
    char    socket_path[256];
    size_t  max_session_bytes;
    int     max_age_days;
    int     fail_closed;
    int     gzip_on_rotate;
    int     chattr_append_only;
    int     log_level;
} pmp_daemon_config_t;
pmp_daemon_config_t *pmp_config_get(void);

static volatile sig_atomic_t g_running = 1;

static void sig_handler(int sig)
{
    (void)sig;
    g_running = 0;
}

static int create_dirs(const char *storage_dir)
{
    char path[512];
    struct stat st;

    if (stat(storage_dir, &st) < 0) {
        if (mkdir(storage_dir, 0750) < 0 && errno != EEXIST) {
            PMP_LOG_ERR("mkdir %s: %s", storage_dir, strerror(errno));
            return -1;
        }
    }

    snprintf(path, sizeof(path), "%s", PMP_SESSIONS_DIR);
    if (mkdir(PMP_RUN_DIR, 0755) < 0 && errno != EEXIST) {}
    if (mkdir(path, 0755) < 0 && errno != EEXIST) {}

    return 0;
}

int main(int argc, char *argv[])
{
    const char *conf_path = NULL;
    int opt;

    while ((opt = getopt(argc, argv, "c:dh")) != -1) {
        switch (opt) {
        case 'c': conf_path = optarg; break;
        case 'd': break; /* daemonize — handled by systemd */
        case 'h':
            fprintf(stderr, "Usage: pmp-recd [-c config]\n");
            return 0;
        }
    }

    pmp_config_load(conf_path);
    pmp_daemon_config_t *cfg = pmp_config_get();

    pmp_log_init("pmp-recd", 0, cfg->log_level);

    signal(SIGTERM, sig_handler);
    signal(SIGINT,  sig_handler);
    signal(SIGPIPE, SIG_IGN);

    if (create_dirs(cfg->storage_dir) < 0)
        return 1;

    int listen_fd;

#ifdef HAVE_SYSTEMD
    /* Socket activation: systemd may hand us a pre-bound socket fd. */
    {
        int n = sd_listen_fds(0);
        if (n > 0) {
            listen_fd = SD_LISTEN_FDS_START;
            PMP_LOG_INFO("using systemd socket-activated fd %d", listen_fd);
        } else {
            listen_fd = pmp_server_bind(cfg->socket_path, 0666);
            if (listen_fd < 0) {
                PMP_LOG_ERR("bind %s: %s", cfg->socket_path, strerror(errno));
                return 1;
            }
        }
    }
#else
    listen_fd = pmp_server_bind(cfg->socket_path, 0666);
    if (listen_fd < 0) {
        PMP_LOG_ERR("bind %s: %s", cfg->socket_path, strerror(errno));
        return 1;
    }
#endif

    int epoll_fd = epoll_create1(EPOLL_CLOEXEC);
    if (epoll_fd < 0) {
        PMP_LOG_ERR("epoll_create1: %s", strerror(errno));
        return 1;
    }

    struct epoll_event ev;
    ev.events  = EPOLLIN;
    ev.data.fd = listen_fd;
    epoll_ctl(epoll_fd, EPOLL_CTL_ADD, listen_fd, &ev);

    PMP_LOG_INFO("pmp-recd started, socket=%s storage=%s",
                 cfg->socket_path, cfg->storage_dir);

    sd_notify(0, "READY=1\nSTATUS=Listening");

    #define MAX_EVENTS 64
    struct epoll_event events[MAX_EVENTS];

    while (g_running) {
        /* Ping systemd watchdog every epoll cycle (≤5s) */
        sd_notify(0, "WATCHDOG=1");

        int n = epoll_wait(epoll_fd, events, MAX_EVENTS, 5000 /* ms */);
        if (n < 0) {
            if (errno == EINTR) continue;
            break;
        }

        for (int i = 0; i < n; i++) {
            if (events[i].data.fd == listen_fd) {
                pmp_server_accept(listen_fd, cfg->storage_dir, epoll_fd);
            } else {
                void *c = events[i].data.ptr;
                if (!c) continue;

                if (events[i].events & (EPOLLHUP | EPOLLERR)) {
                    pmp_server_drop_client(c, epoll_fd);
                } else if (events[i].events & EPOLLIN) {
                    pmp_server_handle_client(c, epoll_fd);
                }
            }
        }
    }

    PMP_LOG_INFO("pmp-recd shutting down");
    sd_notify(0, "STOPPING=1");
    close(listen_fd);
    close(epoll_fd);
    unlink(cfg->socket_path);
    return 0;
}
