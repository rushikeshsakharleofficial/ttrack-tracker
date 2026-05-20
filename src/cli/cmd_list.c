#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <dirent.h>
#include <sys/stat.h>
#include <unistd.h>
#include <time.h>
#include <errno.h>

static char *extract_field(const char *src, const char *key)
{
    char search[64];
    snprintf(search, sizeof(search), "\"%s\": \"", key);
    const char *p = strstr(src, search);
    if (!p) {
        snprintf(search, sizeof(search), "\"%s\": ", key);
        p = strstr(src, search);
        if (!p) return strdup("?");
        p += strlen(search);
        const char *end = p;
        while (*end && *end != ',' && *end != '\n' && *end != '}') end++;
        char *r = strndup(p, (size_t)(end - p));
        size_t len = strlen(r);
        if (len > 0 && r[len-1] == '"') r[len-1] = '\0';
        return r;
    }
    p += strlen(search);
    const char *end = strchr(p, '"');
    if (!end) return strdup("?");
    return strndup(p, (size_t)(end - p));
}

static void print_meta(const char *path, const char *filename)
{
    FILE *f = fopen(path, "r");
    if (!f) return;

    char buf[4096];
    size_t n = fread(buf, 1, sizeof(buf)-1, f);
    fclose(f);
    buf[n] = '\0';

    (void)filename;
    char *sid     = extract_field(buf, "sid");
    char *ruser   = extract_field(buf, "ruser");
    char *service = extract_field(buf, "service");
    char *rhost   = extract_field(buf, "rhost");
    char *start   = extract_field(buf, "start_ts");
    char *end_ts  = extract_field(buf, "end_ts");
    char *exit_s  = extract_field(buf, "exit_status");

    /* rhost is SSH_CLIENT format "IP port port" — show IP only */
    char *sp = strchr(rhost, ' ');
    if (sp) *sp = '\0';

    /* Truncate start_ts to 19 chars (drop trailing Z for display) */
    if (strlen(start) > 19) start[19] = '\0';

    const char *status = (strcmp(end_ts, "?") == 0 || strcmp(end_ts, "null") == 0)
                         ? "ACTIVE" : "EXITED";

    printf("%-8s %-37s %-20s %-8s %-16s %-20s %-6s\n",
           status, sid, ruser, service, rhost, start, exit_s);

    free(sid); free(ruser); free(service);
    free(rhost); free(start); free(end_ts); free(exit_s);
}

int cmd_list(int argc, char *argv[])
{
    const char *storage_dir = "/var/lib/pmp-rec";
    const char *filter_user = NULL;

    for (int i = 0; i < argc - 1; i++) {
        if (strcmp(argv[i], "--user") == 0 || strcmp(argv[i], "-u") == 0)
            filter_user = argv[i+1];
        else if (strcmp(argv[i], "--dir") == 0)
            storage_dir = argv[i+1];
    }

    printf("%-8s %-37s %-20s %-8s %-16s %-20s %-6s\n",
           "STATUS", "SID", "USER", "SERVICE", "RHOST", "START", "EXIT");
    printf("%s\n", "---------------------------------------------"
                   "---------------------------------------------"
                   "--------");

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
        DIR *d2 = opendir(datedir);
        if (!d2) continue;

        struct dirent *de2;
        while ((de2 = readdir(d2))) {
            if (!strstr(de2->d_name, ".meta.json")) continue;

            char mpath[1024];
            snprintf(mpath, sizeof(mpath), "%s/%s", datedir, de2->d_name);

            if (filter_user) {
                FILE *ff = fopen(mpath, "r");
                if (!ff) continue;
                char fbuf[2048];
                size_t fn = fread(fbuf, 1, sizeof(fbuf)-1, ff);
                fclose(ff);
                fbuf[fn] = '\0';
                char needle[64];
                snprintf(needle, sizeof(needle), "\"ruser\": \"%s\"", filter_user);
                if (!strstr(fbuf, needle)) continue;
            }

            print_meta(mpath, de2->d_name);
        }
        closedir(d2);
    }
    closedir(top);
    return 0;
}
