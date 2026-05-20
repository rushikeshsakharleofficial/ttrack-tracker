#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <dirent.h>
#include <sys/stat.h>
#include <time.h>
#include <errno.h>

int cmd_purge(int argc, char *argv[])
{
    const char *storage_dir = "/var/lib/trackterm-rec";
    int max_age_days = 90;
    const char *del_sid = NULL;
    int dry_run = 0;

    for (int i = 0; i < argc - 1; i++) {
        if (strcmp(argv[i], "--older-than") == 0)
            max_age_days = atoi(argv[i+1]);
        else if (strcmp(argv[i], "--sid") == 0)
            del_sid = argv[i+1];
        else if (strcmp(argv[i], "--dir") == 0)
            storage_dir = argv[i+1];
        else if (strcmp(argv[i], "--dry-run") == 0)
            dry_run = 1;
    }

    if (del_sid) {
        /* Delete all files for a specific session */
        DIR *top = opendir(storage_dir);
        if (!top) return 1;
        struct dirent *de;
        while ((de = readdir(top))) {
            if (de->d_name[0] == '.') continue;
            char datedir[512];
            snprintf(datedir, sizeof(datedir), "%s/%s", storage_dir, de->d_name);
            DIR *d2 = opendir(datedir);
            if (!d2) continue;
            struct dirent *de2;
            while ((de2 = readdir(d2))) {
                if (strncmp(de2->d_name, del_sid, 36) != 0) continue;
                char fp[1024];
                snprintf(fp, sizeof(fp), "%s/%s", datedir, de2->d_name);
                if (dry_run) printf("would delete: %s\n", fp);
                else { unlink(fp); printf("deleted: %s\n", fp); }
            }
            closedir(d2);
        }
        closedir(top);
        return 0;
    }

    time_t cutoff = time(NULL) - (time_t)max_age_days * 86400;
    DIR *top = opendir(storage_dir);
    if (!top) {
        fprintf(stderr, "opendir %s: %s\n", storage_dir, strerror(errno));
        return 1;
    }

    struct dirent *de;
    while ((de = readdir(top))) {
        if (de->d_name[0] == '.') continue;
        char datedir[512];
        snprintf(datedir, sizeof(datedir), "%s/%s", storage_dir, de->d_name);

        struct stat st;
        if (stat(datedir, &st) < 0 || !S_ISDIR(st.st_mode)) continue;
        if (st.st_mtime >= cutoff) continue;

        DIR *d2 = opendir(datedir);
        if (!d2) continue;
        struct dirent *de2;
        while ((de2 = readdir(d2))) {
            if (de2->d_name[0] == '.') continue;
            char fp[1024];
            snprintf(fp, sizeof(fp), "%s/%s", datedir, de2->d_name);
            if (dry_run) printf("would delete: %s\n", fp);
            else unlink(fp);
        }
        closedir(d2);
        if (!dry_run) {
            rmdir(datedir);
            printf("purged: %s\n", datedir);
        }
    }
    closedir(top);
    return 0;
}
