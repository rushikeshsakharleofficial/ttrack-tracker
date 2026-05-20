#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <errno.h>
#include <time.h>
#include "pmp_log.h"

/* Recursively create directory (mkdir -p). */
static int mkdirp(const char *path, mode_t mode)
{
    char tmp[512];
    char *p;
    size_t len;

    strncpy(tmp, path, sizeof(tmp) - 1);
    len = strlen(tmp);
    if (len == 0) return -1;
    if (tmp[len-1] == '/') tmp[len-1] = '\0';

    for (p = tmp + 1; *p; p++) {
        if (*p == '/') {
            *p = '\0';
            if (mkdir(tmp, mode) < 0 && errno != EEXIST) return -1;
            *p = '/';
        }
    }
    if (mkdir(tmp, mode) < 0 && errno != EEXIST) return -1;
    return 0;
}

/*
 * Build per-session file paths:
 *   <storage_dir>/<YYYY-MM-DD>/<sid>.<ext>
 * Creates directories as needed.
 */
int pmp_paths_build(const char *storage_dir, const char *sid,
                    const char *ext, char *out, size_t outsz)
{
    time_t t = time(NULL);
    struct tm *tm = gmtime(&t);
    char date[16];
    char dir[512];

    strftime(date, sizeof(date), "%Y-%m-%d", tm);
    snprintf(dir, sizeof(dir), "%s/%s", storage_dir, date);

    if (mkdirp(dir, 0750) < 0) {
        PMP_LOG_ERR("mkdirp %s: %s", dir, strerror(errno));
        return -1;
    }

    snprintf(out, outsz, "%s/%s.%s", dir, sid, ext);
    return 0;
}
