#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <fcntl.h>
#include <time.h>
#include <errno.h>
#include "trackterm_proto.h"
#include "trackterm_log.h"

/* Escape a string for safe embedding inside a JSON double-quoted value.
 * Handles backslash, double-quote, and ASCII control characters. */
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

int trackterm_meta_write(int fd, const struct trackterm_hello *h, time_t start_ts)
{
    char buf[2048];
    char start_iso[32];
    struct tm *tm;

    /* Escape all user-controlled string fields before embedding in JSON */
    char e_sid[128], e_parent[128], e_service[128], e_tty[256];
    char e_term[128], e_rhost[256], e_ruser[128], e_hostname[256];
    json_escape(h->sid,        e_sid,      sizeof(e_sid));
    json_escape(h->parent_sid, e_parent,   sizeof(e_parent));
    json_escape(h->service,    e_service,  sizeof(e_service));
    json_escape(h->tty,        e_tty,      sizeof(e_tty));
    json_escape(h->term,       e_term,     sizeof(e_term));
    json_escape(h->rhost,      e_rhost,    sizeof(e_rhost));
    json_escape(h->ruser,      e_ruser,    sizeof(e_ruser));
    json_escape(h->hostname,   e_hostname, sizeof(e_hostname));

    tm = gmtime(&start_ts);
    strftime(start_iso, sizeof(start_iso), "%Y-%m-%dT%H:%M:%SZ", tm);

    int n = snprintf(buf, sizeof(buf),
        "{\n"
        "  \"sid\": \"%s\",\n"
        "  \"parent_sid\": \"%s\",\n"
        "  \"loginuid\": %u,\n"
        "  \"euid\": %u,\n"
        "  \"gid\": %u,\n"
        "  \"service\": \"%s\",\n"
        "  \"tty\": \"%s\",\n"
        "  \"term\": \"%s\",\n"
        "  \"rhost\": \"%s\",\n"
        "  \"ruser\": \"%s\",\n"
        "  \"hostname\": \"%s\",\n"
        "  \"rows\": %u,\n"
        "  \"cols\": %u,\n"
        "  \"start_ts\": \"%s\",\n"
        "  \"end_ts\": null,\n"
        "  \"exit_status\": null,\n"
        "  \"bytes_recorded\": 0\n"
        "}\n",
        e_sid, e_parent,
        h->loginuid, h->euid, h->gid,
        e_service, e_tty, e_term,
        e_rhost, e_ruser, e_hostname,
        h->rows, h->cols,
        start_iso);

    if (n <= 0) return -1;

    /* Truncate and rewrite */
    ftruncate(fd, 0);
    lseek(fd, 0, SEEK_SET);

    const char *p = buf;
    size_t rem = (size_t)n;
    while (rem > 0) {
        ssize_t r = write(fd, p, rem);
        if (r < 0) { if (errno == EINTR) continue; return -1; }
        p += r; rem -= (size_t)r;
    }
    return 0;
}

int trackterm_meta_finalize(int fd, time_t end_ts, int exit_status,
                      uint64_t bytes_recorded)
{
    /* Seek back to update end_ts, exit_status, bytes_recorded.
     * Simple approach: re-read, patch JSON fields, rewrite.
     * For production, a proper JSON patcher would be used.
     * Here we do a string replace on the known null fields.
     */
    char buf[2048];
    ssize_t n;
    char end_iso[32];
    struct tm *tm;

    lseek(fd, 0, SEEK_SET);
    n = read(fd, buf, sizeof(buf) - 1);
    if (n <= 0) return -1;
    buf[n] = '\0';

    tm = gmtime(&end_ts);
    strftime(end_iso, sizeof(end_iso), "%Y-%m-%dT%H:%M:%SZ", tm);

    /* Patch null → values */
    char *p;
    char patch[64];

    p = strstr(buf, "\"end_ts\": null");
    if (p) {
        char repl[64];
        snprintf(repl, sizeof(repl), "\"end_ts\": \"%s\"", end_iso);
        size_t old_len = strlen("\"end_ts\": null");
        size_t new_len = strlen(repl);
        memmove(p + new_len, p + old_len, strlen(p + old_len) + 1);
        memcpy(p, repl, new_len);
    }

    p = strstr(buf, "\"exit_status\": null");
    if (p) {
        snprintf(patch, sizeof(patch), "\"exit_status\": %d       ", exit_status);
        memcpy(p, patch, strlen("\"exit_status\": null"));
    }

    p = strstr(buf, "\"bytes_recorded\": 0");
    if (p) {
        snprintf(patch, sizeof(patch), "\"bytes_recorded\": %llu",
                 (unsigned long long)bytes_recorded);
        size_t old_len = strlen("\"bytes_recorded\": 0");
        size_t new_len = strlen(patch);
        if (new_len <= old_len)
            memcpy(p, patch, new_len);
    }

    ftruncate(fd, 0);
    lseek(fd, 0, SEEK_SET);
    n = (ssize_t)strlen(buf);
    write(fd, buf, (size_t)n);
    fsync(fd);

    return 0;
}
