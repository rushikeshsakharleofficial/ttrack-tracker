#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <dirent.h>
#include <errno.h>
#include <sys/stat.h>
#include <time.h>
#include <pthread.h>
#include <fcntl.h>
#include <zlib.h>
#include <linux/fs.h>
#include <sys/ioctl.h>
#include "pmp_log.h"

static void gzip_file(const char *src_path)
{
    char dst_path[512];
    snprintf(dst_path, sizeof(dst_path), "%s.gz", src_path);

    FILE *src = fopen(src_path, "rb");
    if (!src) return;

    gzFile dst = gzopen(dst_path, "wb6");
    if (!dst) { fclose(src); return; }

    char buf[65536];
    size_t n;
    while ((n = fread(buf, 1, sizeof(buf), src)) > 0)
        gzwrite(dst, buf, (unsigned)n);

    fclose(src);
    gzclose(dst);
    unlink(src_path);

    PMP_LOG_INFO("rotated %s → %s", src_path, dst_path);
}

typedef struct {
    char path[512];
} rotate_job_t;

static void *rotate_thread(void *arg)
{
    rotate_job_t *job = arg;
    gzip_file(job->path);
    free(job);
    return NULL;
}

void pmp_rotate_file_async(const char *path)
{
    rotate_job_t *job = malloc(sizeof(*job));
    if (!job) return;
    strncpy(job->path, path, sizeof(job->path) - 1);

    pthread_t t;
    pthread_attr_t attr;
    pthread_attr_init(&attr);
    pthread_attr_setdetachstate(&attr, PTHREAD_CREATE_DETACHED);
    pthread_create(&t, &attr, rotate_thread, job);
    pthread_attr_destroy(&attr);
}

/* Walk storage_dir, delete .ttyrec.gz + sidecar files older than max_age_days. */
void pmp_purge_old_sessions(const char *storage_dir, int max_age_days)
{
    time_t cutoff = time(NULL) - (time_t)max_age_days * 86400;
    DIR *top = opendir(storage_dir);
    if (!top) return;

    struct dirent *de;
    while ((de = readdir(top))) {
        if (de->d_name[0] == '.') continue;

        char datedir[512];
        snprintf(datedir, sizeof(datedir), "%s/%s", storage_dir, de->d_name);

        struct stat st;
        if (stat(datedir, &st) < 0 || !S_ISDIR(st.st_mode)) continue;
        if (st.st_mtime >= cutoff) continue;

        /* Delete everything in this date dir */
        DIR *d2 = opendir(datedir);
        if (!d2) continue;

        struct dirent *de2;
        while ((de2 = readdir(d2))) {
            if (de2->d_name[0] == '.') continue;
            char fp[1024];
            snprintf(fp, sizeof(fp), "%s/%s", datedir, de2->d_name);
            unlink(fp);
        }
        closedir(d2);
        rmdir(datedir);

        PMP_LOG_INFO("purged date dir %s", datedir);
    }
    closedir(top);
}

/* Set FS_APPEND_FL (chattr +a) on a closed session file.
 * Forensic friction: even root must explicitly clear the flag to truncate.
 * Silently skips on non-ext4/xfs (EOPNOTSUPP) or permission errors. */
void pmp_chattr_append_only(const char *path)
{
    int fd = open(path, O_RDONLY | O_CLOEXEC);
    if (fd < 0) return;

    int flags = 0;
    if (ioctl(fd, FS_IOC_GETFLAGS, &flags) == 0) {
        flags |= FS_APPEND_FL;
        if (ioctl(fd, FS_IOC_SETFLAGS, &flags) < 0 && errno != EOPNOTSUPP)
            PMP_LOG_WARN("chattr +a %s: %s", path, strerror(errno));
    }
    close(fd);
}
