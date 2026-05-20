#define _GNU_SOURCE
#include <stdio.h>
#include <fcntl.h>
#include <unistd.h>
#include <string.h>
#include <errno.h>
#include "pmp_uuid.h"

int pmp_uuid_generate(char buf[37])
{
    static const char kernel_uuid[] = "/proc/sys/kernel/random/uuid";
    int fd;
    ssize_t n;

    fd = open(kernel_uuid, O_RDONLY | O_CLOEXEC);
    if (fd < 0)
        return -errno;

    n = read(fd, buf, 36);
    close(fd);

    if (n != 36)
        return -EIO;

    buf[36] = '\0';
    /* strip trailing newline if present */
    if (buf[35] == '\n')
        buf[35] = '\0';

    return 0;
}
