#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <ctype.h>
#include "trackterm_log.h"
#include "trackterm_paths.h"

#define MAXLINE 512

typedef struct trackterm_daemon_config {
    char    storage_dir[256];
    char    socket_path[256];
    size_t  max_session_bytes;    /* per-session file size before rotation */
    int     max_age_days;
    int     fail_closed;
    int     gzip_on_rotate;
    int     chattr_append_only;
    int     log_level;
} trackterm_daemon_config_t;

static trackterm_daemon_config_t g_cfg = {
    .storage_dir       = "/var/lib/trackterm-rec",
    .socket_path       = "/run/trackterm-recd.sock",
    .max_session_bytes = 64 * 1024 * 1024,   /* 64 MiB */
    .max_age_days      = 90,
    .fail_closed       = 0,
    .gzip_on_rotate    = 1,
    .chattr_append_only= 0,
    .log_level         = 6, /* LOG_INFO */
};

trackterm_daemon_config_t *trackterm_config_get(void)
{
    return &g_cfg;
}

static void trim(char *s)
{
    size_t n = strlen(s);
    while (n > 0 && isspace((unsigned char)s[n-1])) s[--n] = '\0';
    while (*s && isspace((unsigned char)*s)) memmove(s, s+1, strlen(s));
}

int trackterm_config_load(const char *path)
{
    FILE *f = fopen(path ? path : TRACKTERM_CONF_RECD, "r");
    if (!f) return 0; /* missing config is fine */

    char line[MAXLINE];
    while (fgets(line, sizeof(line), f)) {
        char *eq = strchr(line, '=');
        if (!eq) continue;
        *eq = '\0';
        char *key = line, *val = eq + 1;
        trim(key); trim(val);
        if (key[0] == '#' || key[0] == '\0') continue;

        if      (!strcmp(key, "storage_dir"))
            strncpy(g_cfg.storage_dir, val, sizeof(g_cfg.storage_dir) - 1);
        else if (!strcmp(key, "socket_path"))
            strncpy(g_cfg.socket_path, val, sizeof(g_cfg.socket_path) - 1);
        else if (!strcmp(key, "max_session_mb"))
            g_cfg.max_session_bytes = (size_t)atoi(val) * 1024 * 1024;
        else if (!strcmp(key, "max_age_days"))
            g_cfg.max_age_days = atoi(val);
        else if (!strcmp(key, "fail_closed"))
            g_cfg.fail_closed = atoi(val);
        else if (!strcmp(key, "gzip_on_rotate"))
            g_cfg.gzip_on_rotate = atoi(val);
        else if (!strcmp(key, "chattr_append_only"))
            g_cfg.chattr_append_only = atoi(val);
    }

    fclose(f);
    return 0;
}
