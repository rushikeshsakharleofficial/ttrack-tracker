#define _GNU_SOURCE
#include <time.h>
#include <stdint.h>
#include <string.h>
#include <unistd.h>
#include <errno.h>
#include <stdio.h>
#include "pmp_ttyrec.h"

static int write_all(int fd, const void *buf, size_t n)
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

int ttyrec_write_frame(int fd, const void *data, uint32_t len)
{
    struct timespec ts;
    struct ttyrec_hdr h;

    clock_gettime(CLOCK_REALTIME, &ts);

    h.tv_sec  = (uint32_t)ts.tv_sec;
    h.tv_usec = (uint32_t)(ts.tv_nsec / 1000);
    h.len     = len;

    if (write_all(fd, &h, sizeof(h)) < 0)
        return -1;
    if (len > 0 && write_all(fd, data, len) < 0)
        return -1;

    return 0;
}

int ttyrec_write_event(int fd, double t_sec, const char *json_body)
{
    char line[1024];
    int n = snprintf(line, sizeof(line), "{\"t\":%.6f,%s}\n", t_sec, json_body);
    if (n <= 0 || (size_t)n >= sizeof(line))
        return -1;
    return write_all(fd, line, (size_t)n);
}
