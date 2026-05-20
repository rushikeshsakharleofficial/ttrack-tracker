#define _GNU_SOURCE
#include <pty.h>
#include <utmp.h>
#include <termios.h>
#include <sys/ioctl.h>
#include <unistd.h>
#include <stdlib.h>
#include <string.h>
#include <errno.h>
#include <fcntl.h>
#include "trackterm_log.h"

int trackterm_pty_open(int *amaster, int *aslave, char **slave_name,
                 const struct winsize *winp)
{
    int master, slave;
    char namebuf[256];

    if (openpty(&master, &slave, namebuf, NULL, winp) < 0)
        return -errno;

    /* Set CLOEXEC on master — slave stays open in child only */
    fcntl(master, F_SETFD, FD_CLOEXEC);

    if (slave_name) {
        *slave_name = strdup(namebuf);
        if (!*slave_name) {
            close(master); close(slave);
            return -ENOMEM;
        }
    }

    *amaster = master;
    *aslave  = slave;
    return 0;
}

int trackterm_pty_child_setup(int slave_fd)
{
    /* login_tty: setsid, TIOCSCTTY, dup2(slave,0/1/2), close slave */
    if (login_tty(slave_fd) < 0)
        return -errno;
    return 0;
}

int trackterm_pty_set_raw(int fd, struct termios *saved)
{
    struct termios raw;

    if (tcgetattr(fd, saved) < 0)
        return -errno;

    raw = *saved;
    cfmakeraw(&raw);
    raw.c_cc[VMIN]  = 1;
    raw.c_cc[VTIME] = 0;

    if (tcsetattr(fd, TCSANOW, &raw) < 0)
        return -errno;

    return 0;
}

int trackterm_pty_restore(int fd, const struct termios *saved)
{
    if (tcsetattr(fd, TCSANOW, saved) < 0)
        return -errno;
    return 0;
}

int trackterm_pty_get_winsize(int fd, struct winsize *ws)
{
    if (ioctl(fd, TIOCGWINSZ, ws) < 0)
        return -errno;
    return 0;
}

int trackterm_pty_set_winsize(int master_fd, const struct winsize *ws)
{
    if (ioctl(master_fd, TIOCSWINSZ, ws) < 0)
        return -errno;
    return 0;
}
