#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <dirent.h>
#include <errno.h>

typedef struct sess_node {
    char sid[37];
    char parent_sid[37];
    char ruser[32];
    char service[32];
    char start_ts[32];
} sess_node_t;

#define MAX_NODES 1024
static sess_node_t g_nodes[MAX_NODES];
static int g_n = 0;

static void load_meta(const char *path)
{
    if (g_n >= MAX_NODES) return;

    FILE *f = fopen(path, "r");
    if (!f) return;
    char buf[4096];
    size_t n = fread(buf, 1, sizeof(buf)-1, f);
    fclose(f);
    buf[n] = '\0';

    sess_node_t *node = &g_nodes[g_n];
    memset(node, 0, sizeof(*node));

    auto void extr(const char *src, const char *key, char *out, size_t outsz);
    void extr(const char *src, const char *key, char *out, size_t outsz) {
        char search[64];
        snprintf(search, sizeof(search), "\"%s\": \"", key);
        const char *p = strstr(src, search);
        if (!p) return;
        p += strlen(search);
        const char *end = strchr(p, '"');
        if (!end) return;
        size_t len = (size_t)(end - p);
        if (len >= outsz) len = outsz - 1;
        memcpy(out, p, len);
        out[len] = '\0';
    }

    extr(buf, "sid",        node->sid,       sizeof(node->sid));
    extr(buf, "parent_sid", node->parent_sid, sizeof(node->parent_sid));
    extr(buf, "ruser",      node->ruser,     sizeof(node->ruser));
    extr(buf, "service",    node->service,   sizeof(node->service));
    extr(buf, "start_ts",   node->start_ts,  sizeof(node->start_ts));

    if (node->sid[0]) g_n++;
}

static void print_tree(const char *sid, int depth)
{
    for (int i = 0; i < g_n; i++) {
        if (strcmp(g_nodes[i].sid, sid) != 0) continue;
        for (int d = 0; d < depth; d++) printf("  ");
        printf("├─ %s  [%s@%s  %s]\n",
               g_nodes[i].sid, g_nodes[i].ruser,
               g_nodes[i].service, g_nodes[i].start_ts);
        /* Find children */
        for (int j = 0; j < g_n; j++) {
            if (strcmp(g_nodes[j].parent_sid, sid) == 0)
                print_tree(g_nodes[j].sid, depth + 1);
        }
        return;
    }
}

int cmd_tree(int argc, char *argv[])
{
    const char *root_sid = NULL;
    const char *storage_dir = "/var/lib/trackterm-rec";

    for (int i = 0; i < argc; i++) {
        if (strcmp(argv[i], "--dir") == 0 && i+1 < argc)
            storage_dir = argv[++i];
        else if (argv[i][0] != '-')
            root_sid = argv[i];
    }

    if (!root_sid) {
        fprintf(stderr, "Usage: trackterm-cli tree [--dir D] <root-sid>\n");
        return 1;
    }

    /* Load all meta files */
    DIR *top = opendir(storage_dir);
    if (!top) { fprintf(stderr, "opendir %s: %s\n", storage_dir, strerror(errno)); return 1; }
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
            load_meta(mpath);
        }
        closedir(d2);
    }
    closedir(top);

    print_tree(root_sid, 0);
    return 0;
}
