#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <fcntl.h>
#include <errno.h>
#include <time.h>
#include <stdint.h>
#include <endian.h>
#include <dirent.h>
#include <sys/stat.h>
#include "trackterm_ttyrec.h"

/* If arg looks like a SID (not a path), search storage_dir for matching .ttyrec.
 * Returns malloc'd path or NULL. */
static char *find_by_sid(const char *sid, const char *storage_dir)
{
    char pattern[64];
    snprintf(pattern, sizeof(pattern), "%s.ttyrec", sid);

    DIR *top = opendir(storage_dir);
    if (!top) return NULL;

    struct dirent *de;
    char *found = NULL;
    while ((de = readdir(top)) && !found) {
        if (de->d_name[0] == '.') continue;
        char datedir[512];
        snprintf(datedir, sizeof(datedir), "%s/%s", storage_dir, de->d_name);
        DIR *d2 = opendir(datedir);
        if (!d2) continue;
        struct dirent *de2;
        while ((de2 = readdir(d2))) {
            if (strcmp(de2->d_name, pattern) == 0) {
                char fp[1024];
                snprintf(fp, sizeof(fp), "%s/%s", datedir, de2->d_name);
                found = strdup(fp);
                break;
            }
        }
        closedir(d2);
    }
    closedir(top);
    return found;
}

static int write_all(int fd, const void *buf, size_t n)
{
    const uint8_t *p = buf;
    while (n > 0) {
        ssize_t r = write(fd, p, n);
        if (r < 0) { if (errno == EINTR) continue; return -1; }
        p += r; n -= (size_t)r;
    }
    return 0;
}

static void nano_sleep_us(long us)
{
    if (us <= 0) return;
    struct timespec ts = { .tv_sec = us / 1000000, .tv_nsec = (us % 1000000) * 1000 };
    nanosleep(&ts, NULL);
}

int cmd_play(int argc, char *argv[])
{
    const char *ttyrec_path = NULL;
    const char *storage_dir = "/var/lib/trackterm-rec";
    double speed = 1.0;
    int noplay = 0;
    int force = 0;
    char *resolved_path = NULL;

    for (int i = 0; i < argc; i++) {
        if (strcmp(argv[i], "--speed") == 0 && i+1 < argc)
            speed = atof(argv[++i]);
        else if (strcmp(argv[i], "--dump") == 0)
            noplay = 1;
        else if (strcmp(argv[i], "--force") == 0)
            force = 1;
        else if (strcmp(argv[i], "--dir") == 0 && i+1 < argc)
            storage_dir = argv[++i];
        else if (argv[i][0] != '-')
            ttyrec_path = argv[i];
    }

    if (!ttyrec_path) {
        fprintf(stderr, "Usage: trackterm-cli play [--speed N] [--dump] [--force] [--dir D] <sid|file.ttyrec>\n");
        return 1;
    }

    /* Guard: refuse to play the session currently recording THIS terminal.
     * Replaying into the same recording causes the playback output to be
     * captured, growing the ttyrec file, which play then reads again — loop. */
    if (!force && ttyrec_path[0] != '/') {
        const char *cur_sid = getenv("TRACKTERM_REC_SID");
        if (cur_sid && strncmp(cur_sid, ttyrec_path, 36) == 0) {
            char cur_tty[64] = "?";
            char *t = ttyname(STDOUT_FILENO);
            if (!t) t = ttyname(STDIN_FILENO);
            if (t) snprintf(cur_tty, sizeof(cur_tty), "%s", t);
            fprintf(stderr,
                "error: session %s is the active recording for this terminal (%s).\n"
                "Playing it here would loop — replay output gets recorded,\n"
                "growing the file, which play reads again on next run.\n"
                "Play from a different terminal, or use --force to override.\n",
                ttyrec_path, cur_tty);
            return 1;
        }
    }

    /* If not an absolute path, search by SID */
    if (ttyrec_path[0] != '/') {
        resolved_path = find_by_sid(ttyrec_path, storage_dir);
        if (!resolved_path) {
            fprintf(stderr, "session %s not found in %s\n", ttyrec_path, storage_dir);
            return 1;
        }
        ttyrec_path = resolved_path;
    }

    /* Guard: warn if session is still active (end_ts not set in meta.json).
     * Playing an active session from a DIFFERENT terminal is allowed but warned. */
    if (!force) {
        /* Find matching meta.json in same dir as the ttyrec */
        char meta_path[1024];
        snprintf(meta_path, sizeof(meta_path), "%s", ttyrec_path);
        char *dot = strstr(meta_path, ".ttyrec");
        if (dot) {
            strcpy(dot, ".meta.json");
            FILE *mf = fopen(meta_path, "r");
            if (mf) {
                char mbuf[512];
                size_t mn = fread(mbuf, 1, sizeof(mbuf)-1, mf);
                fclose(mf);
                mbuf[mn] = '\0';
                if (strstr(mbuf, "\"end_ts\": null")) {
                    char cur_tty[64] = "?";
                    char *t = ttyname(STDOUT_FILENO);
                    if (!t) t = ttyname(STDIN_FILENO);
                    if (t) snprintf(cur_tty, sizeof(cur_tty), "%s", t);
                    fprintf(stderr,
                        "warning: session is still ACTIVE (being recorded).\n"
                        "Playing from this terminal (%s) while recording may cause a loop.\n"
                        "Use --force to play anyway.\n", cur_tty);
                    free(resolved_path);
                    return 1;
                }
            }
        }
    }

    int fd = open(ttyrec_path, O_RDONLY | O_CLOEXEC);
    if (fd < 0) {
        fprintf(stderr, "open %s: %s\n", ttyrec_path, strerror(errno));
        free(resolved_path);
        return 1;
    }

    /* Snapshot file size at open — prevents infinite loop when playing an
     * active session (trackterm-rec would record replay output, growing the file). */
    struct stat st;
    off_t play_limit = -1;
    if (fstat(fd, &st) == 0)
        play_limit = st.st_size;

    uint32_t prev_sec = 0, prev_usec = 0;
    int first = 1;
    uint8_t *payload = NULL;
    size_t payload_cap = 0;
    off_t bytes_read = 0;

    for (;;) {
        if (play_limit >= 0 && bytes_read >= play_limit) break;

        struct ttyrec_hdr h;
        ssize_t n = read(fd, &h, sizeof(h));
        if (n == 0) break;
        if (n < (ssize_t)sizeof(h)) {
            fprintf(stderr, "short read on header\n");
            break;
        }
        bytes_read += n;

        /* headers are already LE uint32 on little-endian host — use directly */
        uint32_t tv_sec  = le32toh(h.tv_sec);
        uint32_t tv_usec = le32toh(h.tv_usec);
        uint32_t len     = le32toh(h.len);

        if (len > 0) {
            if (len > payload_cap) {
                free(payload);
                payload = malloc(len + 1);
                if (!payload) break;
                payload_cap = len + 1;
            }
            n = read(fd, payload, len);
            if (n < (ssize_t)len) break;
            bytes_read += n;
        }

        if (!noplay && !first) {
            long delay_us = (long)(tv_sec - prev_sec) * 1000000L
                          + (long)tv_usec - (long)prev_usec;
            if (speed > 0 && speed != 1.0)
                delay_us = (long)((double)delay_us / speed);
            if (delay_us > 5000000L) delay_us = 5000000L; /* max 5s */
            nano_sleep_us(delay_us);
        }

        if (len > 0)
            write_all(STDOUT_FILENO, payload, len);

        prev_sec  = tv_sec;
        prev_usec = tv_usec;
        first = 0;
    }

    free(payload);
    close(fd);
    return 0;
}
