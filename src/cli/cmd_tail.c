#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <fcntl.h>
#include <dirent.h>
#include <errno.h>
#include <sys/stat.h>
#include <signal.h>
#include <stdint.h>
#include <time.h>

static volatile int g_stop = 0;
static void sig_stop(int s) { (void)s; g_stop = 1; }

int cmd_tail(int argc, char *argv[])
{
    const char *sid = NULL;
    const char *storage_dir = "/var/lib/pmp-rec";

    for (int i = 0; i < argc; i++) {
        if (strcmp(argv[i], "--dir") == 0 && i+1 < argc)
            storage_dir = argv[++i];
        else if (argv[i][0] != '-')
            sid = argv[i];
    }

    if (!sid) {
        fprintf(stderr, "Usage: pmp-rec-cli tail [--dir D] <sid>\n");
        return 1;
    }

    /* Find the .ttyrec file */
    char ttyrec_path[1024] = "";
    DIR *top = opendir(storage_dir);
    if (!top) { fprintf(stderr, "opendir %s: %s\n", storage_dir, strerror(errno)); return 1; }

    struct dirent *de;
    while ((de = readdir(top)) && !ttyrec_path[0]) {
        if (de->d_name[0] == '.') continue;
        char datedir[512];
        snprintf(datedir, sizeof(datedir), "%s/%s", storage_dir, de->d_name);
        DIR *d2 = opendir(datedir);
        if (!d2) continue;
        struct dirent *de2;
        while ((de2 = readdir(d2))) {
            char expected[64];
            snprintf(expected, sizeof(expected), "%s.ttyrec", sid);
            if (strcmp(de2->d_name, expected) == 0) {
                snprintf(ttyrec_path, sizeof(ttyrec_path), "%s/%s", datedir, de2->d_name);
                break;
            }
        }
        closedir(d2);
    }
    closedir(top);

    if (!ttyrec_path[0]) {
        fprintf(stderr, "session %s not found\n", sid);
        return 1;
    }

    int fd = open(ttyrec_path, O_RDONLY | O_CLOEXEC);
    if (fd < 0) { fprintf(stderr, "open: %s\n", strerror(errno)); return 1; }

    signal(SIGINT, sig_stop);
    signal(SIGTERM, sig_stop);

    /* Skip to end to show only new data */
    lseek(fd, 0, SEEK_END);

    uint8_t buf[65536];
    while (!g_stop) {
        ssize_t n = read(fd, buf, sizeof(buf));
        if (n > 0) {
            write(STDOUT_FILENO, buf, (size_t)n);
        } else {
            struct timespec ts = { .tv_sec = 0, .tv_nsec = 100000000 }; /* 100ms */
            nanosleep(&ts, NULL);
        }
    }

    close(fd);
    return 0;
}
