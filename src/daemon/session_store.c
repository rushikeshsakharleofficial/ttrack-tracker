#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <fcntl.h>
#include <errno.h>
#include <time.h>
#include <sys/stat.h>

#include "trackterm_proto.h"
#include "trackterm_ttyrec.h"
#include "trackterm_log.h"

static void json_escape(const char *src, char *dst, size_t dstsz)
{
    size_t d = 0;
    for (size_t i = 0; src[i] && d + 2 < dstsz; i++) {
        unsigned char c = (unsigned char)src[i];
        if (c == '\\' || c == '"') {
            if (d + 2 >= dstsz) break;
            dst[d++] = '\\';
            dst[d++] = (char)c;
        } else if (c < 0x20) {
            if (d + 6 >= dstsz) break;
            snprintf(dst + d, dstsz - d, "\\u%04x", (unsigned)c);
            d += 6;
        } else {
            dst[d++] = (char)c;
        }
    }
    dst[d] = '\0';
}

/* Forward declarations */
int trackterm_paths_build(const char *storage_dir, const char *sid,
                    const char *ext, char *out, size_t outsz);
int trackterm_meta_write(int fd, const struct trackterm_hello *h, time_t start_ts);
int trackterm_meta_finalize(int fd, time_t end_ts, int exit_status,
                      uint64_t bytes_recorded);

typedef struct trackterm_session_store {
    char     sid[37];
    int      ttyrec_fd;
    int      meta_fd;
    int      events_fd;
    uint64_t bytes_written;
    time_t   start_ts;
    char     storage_dir[256];
} trackterm_session_store_t;

trackterm_session_store_t *trackterm_store_open(const char *storage_dir,
                                    const struct trackterm_hello *h)
{
    trackterm_session_store_t *s = calloc(1, sizeof(*s));
    if (!s) return NULL;

    strncpy(s->sid, h->sid, 36);
    strncpy(s->storage_dir, storage_dir, sizeof(s->storage_dir) - 1);
    s->start_ts = time(NULL);
    s->ttyrec_fd = -1;
    s->meta_fd   = -1;
    s->events_fd = -1;

    char path[512];

    /* Open ttyrec file */
    if (trackterm_paths_build(storage_dir, h->sid, "ttyrec", path, sizeof(path)) < 0)
        goto err;
    s->ttyrec_fd = open(path, O_WRONLY | O_CREAT | O_TRUNC | O_CLOEXEC, 0640);
    if (s->ttyrec_fd < 0) {
        TRACKTERM_LOG_ERR("open %s: %s", path, strerror(errno));
        goto err;
    }

    /* Open meta file */
    if (trackterm_paths_build(storage_dir, h->sid, "meta.json", path, sizeof(path)) < 0)
        goto err;
    s->meta_fd = open(path, O_RDWR | O_CREAT | O_TRUNC | O_CLOEXEC, 0640);
    if (s->meta_fd < 0) goto err;
    trackterm_meta_write(s->meta_fd, h, s->start_ts);

    /* Open events file */
    if (trackterm_paths_build(storage_dir, h->sid, "events.jsonl", path, sizeof(path)) < 0)
        goto err;
    s->events_fd = open(path, O_WRONLY | O_CREAT | O_TRUNC | O_CLOEXEC, 0640);
    if (s->events_fd < 0) goto err;

    /* Write start event — escape all string fields to prevent JSON injection */
    {
        char e_term[128], e_tty[256], e_sid[128], e_parent[128];
        char e_service[128], e_rhost[256], e_ruser[128];
        json_escape(h->term,       e_term,    sizeof(e_term));
        json_escape(h->tty,        e_tty,     sizeof(e_tty));
        json_escape(h->sid,        e_sid,     sizeof(e_sid));
        json_escape(h->parent_sid, e_parent,  sizeof(e_parent));
        json_escape(h->service,    e_service, sizeof(e_service));
        json_escape(h->rhost,      e_rhost,   sizeof(e_rhost));
        json_escape(h->ruser,      e_ruser,   sizeof(e_ruser));

        char body[512];
        snprintf(body, sizeof(body),
                 "\"type\":\"start\",\"rows\":%u,\"cols\":%u,"
                 "\"term\":\"%s\",\"tty\":\"%s\","
                 "\"sid\":\"%s\",\"parent_sid\":\"%s\","
                 "\"loginuid\":%u,\"euid\":%u,\"service\":\"%s\","
                 "\"rhost\":\"%s\",\"ruser\":\"%s\"",
                 h->rows, h->cols, e_term, e_tty,
                 e_sid, e_parent,
                 h->loginuid, h->euid, e_service,
                 e_rhost, e_ruser);
        ttyrec_write_event(s->events_fd, 0.0, body);
    }

    TRACKTERM_LOG_INFO("session %s opened (loginuid=%u service=%s rhost=%s)",
                 h->sid, h->loginuid, h->service, h->rhost);
    return s;

err:
    if (s->ttyrec_fd >= 0) close(s->ttyrec_fd);
    if (s->meta_fd   >= 0) close(s->meta_fd);
    if (s->events_fd >= 0) close(s->events_fd);
    free(s);
    return NULL;
}

int trackterm_store_write_output(trackterm_session_store_t *s,
                           const uint8_t *data, uint32_t len)
{
    if (s->ttyrec_fd < 0) return -1;
    int r = ttyrec_write_frame(s->ttyrec_fd, data, len);
    if (r == 0) s->bytes_written += len;
    return r;
}

int trackterm_store_write_resize(trackterm_session_store_t *s, double t,
                           uint16_t rows, uint16_t cols)
{
    if (s->events_fd < 0) return -1;
    char body[128];
    snprintf(body, sizeof(body),
             "\"type\":\"resize\",\"rows\":%u,\"cols\":%u", rows, cols);
    return ttyrec_write_event(s->events_fd, t, body);
}

int trackterm_store_write_gap(trackterm_session_store_t *s, double t, double gap_seconds)
{
    if (s->events_fd < 0) return -1;
    char body[128];
    snprintf(body, sizeof(body),
             "\"type\":\"gap\",\"gap_seconds\":%.3f", gap_seconds);
    return ttyrec_write_event(s->events_fd, t, body);
}

void trackterm_store_close(trackterm_session_store_t *s, int exit_status)
{
    if (!s) return;

    double elapsed = (double)(time(NULL) - s->start_ts);

    if (s->events_fd >= 0) {
        char body[64];
        snprintf(body, sizeof(body), "\"type\":\"end\",\"exit\":%d", exit_status);
        ttyrec_write_event(s->events_fd, elapsed, body);
        fsync(s->events_fd);
        close(s->events_fd);
        s->events_fd = -1;
    }

    if (s->ttyrec_fd >= 0) {
        fsync(s->ttyrec_fd);
        close(s->ttyrec_fd);
        s->ttyrec_fd = -1;
    }

    if (s->meta_fd >= 0) {
        trackterm_meta_finalize(s->meta_fd, time(NULL), exit_status, s->bytes_written);
        close(s->meta_fd);
        s->meta_fd = -1;
    }

    TRACKTERM_LOG_INFO("session %s closed (exit=%d bytes=%llu)",
                 s->sid, exit_status,
                 (unsigned long long)s->bytes_written);
    free(s);
}
