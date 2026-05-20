#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

/* Subcommand declarations */
int cmd_list (int argc, char *argv[]);
int cmd_play (int argc, char *argv[]);
int cmd_tail (int argc, char *argv[]);
int cmd_purge(int argc, char *argv[]);
int cmd_tree (int argc, char *argv[]);
int cmd_tui  (int argc, char *argv[]);

static void usage(const char *prog)
{
    fprintf(stderr,
        "Usage: %s <subcommand> [options]\n"
        "\n"
        "Subcommands:\n"
        "  list   [--user U] [--dir D]          List sessions\n"
        "  play   [--speed N] [--dump] <sid|file>  Replay a session\n"
        "  tail   [--dir D] <sid>               Follow active session\n"
        "  purge  [--older-than N] [--sid S]    Delete old sessions\n"
        "  tree   [--dir D] <root-sid>          Show nested session chain\n"
        "  tui    [--dir D]                     Interactive session browser\n"
        "\n",
        prog);
}

int main(int argc, char *argv[])
{
    if (argc < 2) {
        usage(argv[0]);
        return 1;
    }

    const char *sub = argv[1];
    int sub_argc = argc - 2;
    char **sub_argv = argv + 2;

    if (strcmp(sub, "list")  == 0) return cmd_list (sub_argc, sub_argv);
    if (strcmp(sub, "play")  == 0) return cmd_play (sub_argc, sub_argv);
    if (strcmp(sub, "tail")  == 0) return cmd_tail (sub_argc, sub_argv);
    if (strcmp(sub, "purge") == 0) return cmd_purge(sub_argc, sub_argv);
    if (strcmp(sub, "tree")  == 0) return cmd_tree (sub_argc, sub_argv);
    if (strcmp(sub, "tui")   == 0) return cmd_tui  (sub_argc, sub_argv);

    fprintf(stderr, "Unknown subcommand: %s\n\n", sub);
    usage(argv[0]);
    return 1;
}
