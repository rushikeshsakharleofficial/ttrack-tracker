#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <dirent.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <unistd.h>
#include <errno.h>
#include <signal.h>
#include <ncurses.h>

/* ── session record ──────────────────────────────────────────────────────── */

typedef struct {
    char sid[37];
    char user[32];
    char service[16];
    char rhost[40];
    char start[20];
    char status[8];    /* "ACTIVE" or "EXITED" */
    char exit_code[8]; /* "0", "-1", "null", etc. */
    char dir[512];     /* date directory path (for deletion) */
} session_t;

/* ── SIGWINCH flag ───────────────────────────────────────────────────────── */

static volatile int g_resized = 0;
static void handle_sigwinch(int s) { (void)s; g_resized = 1; }

/* ── JSON field extractor ────────────────────────────────────────────────── */

static char *extract_field(const char *src, const char *key)
{
    char search[64];

    /* Try quoted value first: "key": "value" */
    snprintf(search, sizeof(search), "\"%s\": \"", key);
    const char *p = strstr(src, search);
    if (p) {
        p += strlen(search);
        const char *end = strchr(p, '"');
        if (!end) return strdup("?");
        return strndup(p, (size_t)(end - p));
    }

    /* Try bare value: "key": value */
    snprintf(search, sizeof(search), "\"%s\": ", key);
    p = strstr(src, search);
    if (!p) return strdup("?");
    p += strlen(search);
    const char *end = p;
    while (*end && *end != ',' && *end != '\n' && *end != '}') end++;
    char *r = strndup(p, (size_t)(end - p));
    /* strip any trailing quote that snuck in */
    size_t len = strlen(r);
    if (len > 0 && r[len - 1] == '"') r[len - 1] = '\0';
    return r;
}

/* ── qsort comparator: descending by start timestamp ────────────────────── */

static int cmp_session_desc(const void *a, const void *b)
{
    const session_t *sa = (const session_t *)a;
    const session_t *sb = (const session_t *)b;
    /* ISO timestamps sort lexicographically, so reverse strcmp */
    return strcmp(sb->start, sa->start);
}

/* ── load all sessions from storage_dir ─────────────────────────────────── */

static int load_sessions(const char *storage_dir, session_t **out, int *count)
{
    *out   = NULL;
    *count = 0;

    DIR *top = opendir(storage_dir);
    if (!top) return -1;

    session_t *arr  = NULL;
    int        cap  = 0;
    int        n    = 0;

    struct dirent *de;
    while ((de = readdir(top))) {
        if (de->d_name[0] == '.') continue;

        char datedir[512];
        snprintf(datedir, sizeof(datedir), "%s/%s", storage_dir, de->d_name);

        struct stat st;
        if (stat(datedir, &st) != 0 || !S_ISDIR(st.st_mode)) continue;

        DIR *d2 = opendir(datedir);
        if (!d2) continue;

        struct dirent *de2;
        while ((de2 = readdir(d2))) {
            if (!strstr(de2->d_name, ".meta.json")) continue;

            char mpath[1024];
            snprintf(mpath, sizeof(mpath), "%s/%s", datedir, de2->d_name);

            FILE *f = fopen(mpath, "r");
            if (!f) continue;

            char buf[4096];
            size_t nr = fread(buf, 1, sizeof(buf) - 1, f);
            fclose(f);
            buf[nr] = '\0';

            /* Grow array if needed */
            if (n >= cap) {
                cap = cap ? cap * 2 : 64;
                session_t *tmp = realloc(arr, (size_t)cap * sizeof(session_t));
                if (!tmp) { free(arr); closedir(d2); closedir(top); return -1; }
                arr = tmp;
            }

            session_t *s = &arr[n];
            memset(s, 0, sizeof(*s));

            char *sid     = extract_field(buf, "sid");
            char *ruser   = extract_field(buf, "ruser");
            char *service = extract_field(buf, "service");
            char *rhost   = extract_field(buf, "rhost");
            char *start   = extract_field(buf, "start_ts");
            char *end_ts  = extract_field(buf, "end_ts");
            char *exit_s  = extract_field(buf, "exit_status");

            /* rhost: "IP port port" — keep only IP */
            char *sp = strchr(rhost, ' ');
            if (sp) *sp = '\0';

            /* start: keep first 16 chars ("YYYY-MM-DD HH:MM") */
            if (strlen(start) > 16) start[16] = '\0';
            /* replace the 'T' separator with a space for readability */
            char *tee = strchr(start, 'T');
            if (tee) *tee = ' ';

            const char *status_str =
                (strcmp(end_ts, "?") == 0 || strcmp(end_ts, "null") == 0)
                ? "ACTIVE" : "EXITED";

            snprintf(s->sid,      sizeof(s->sid),      "%s", sid);
            snprintf(s->user,     sizeof(s->user),     "%s", ruser);
            snprintf(s->service,  sizeof(s->service),  "%s", service);
            snprintf(s->rhost,    sizeof(s->rhost),    "%s", rhost);
            snprintf(s->start,    sizeof(s->start),    "%s", start);
            snprintf(s->status,   sizeof(s->status),   "%s", status_str);
            snprintf(s->exit_code,sizeof(s->exit_code),"%s", exit_s);
            snprintf(s->dir,      sizeof(s->dir),      "%s", datedir);

            free(sid); free(ruser); free(service);
            free(rhost); free(start); free(end_ts); free(exit_s);

            n++;
        }
        closedir(d2);
    }
    closedir(top);

    if (n > 1)
        qsort(arr, (size_t)n, sizeof(session_t), cmp_session_desc);

    *out   = arr;
    *count = n;
    return 0;
}

/* ── free session array ──────────────────────────────────────────────────── */

static void free_sessions(session_t *arr)
{
    free(arr);
}

/* ── draw the full screen ────────────────────────────────────────────────── */

#define COL_STATUS  0
#define COL_USER    10
#define COL_SERVICE 33
#define COL_RHOST   42
#define COL_START   59

static void draw_screen(session_t *sessions, int count,
                        int selected, int scroll_off,
                        const char *storage_dir)
{
    int rows, cols;
    getmaxyx(stdscr, rows, cols);
    (void)storage_dir;

    clear();

    /* ── title bar ── */
    attron(A_BOLD | A_REVERSE);
    char title[256];
    snprintf(title, sizeof(title),
             " pmp-rec session browser  %d session%s  [q]uit [?]help ",
             count, count == 1 ? "" : "s");
    mvprintw(0, 0, "%-*s", cols, title);
    attroff(A_BOLD | A_REVERSE);

    /* ── column header ── */
    attron(A_BOLD);
    mvprintw(1, 0, "%-*s", cols,
             " STATUS   USER                 SERVICE  RHOST            START           ");
    attroff(A_BOLD);

    /* ── separator ── */
    mvhline(2, 0, ACS_HLINE, cols);

    /* ── list rows ── */
    int list_rows = rows - 5;  /* title + header + sep + sep + status bar */
    if (list_rows < 1) list_rows = 1;

    for (int i = 0; i < list_rows; i++) {
        int idx = scroll_off + i;
        int row = 3 + i;

        if (idx >= count) {
            move(row, 0);
            clrtoeol();
            continue;
        }

        session_t *s = &sessions[idx];
        int is_sel   = (idx == selected);
        int is_active = (strcmp(s->status, "ACTIVE") == 0);

        if (is_sel)   attron(A_REVERSE);
        if (is_active && !is_sel) attron(COLOR_PAIR(1));

        char line[256];
        snprintf(line, sizeof(line),
                 " %-7s  %-20s  %-7s  %-15s  %-16s",
                 s->status,
                 s->user,
                 s->service,
                 s->rhost,
                 s->start);

        mvprintw(row, 0, "%-*s", cols, line);

        if (is_active && !is_sel) attroff(COLOR_PAIR(1));
        if (is_sel)   attroff(A_REVERSE);
    }

    /* ── bottom separator ── */
    mvhline(rows - 2, 0, ACS_HLINE, cols);

    /* ── status / key hints ── */
    attron(A_BOLD);
    mvprintw(rows - 1, 0,
             " [" "\xe2\x86\x91" "\xe2\x86\x93" "/jk] nav"
             "  [p] play  [t] tail  [d] delete  [r] refresh  [q] quit");
    attroff(A_BOLD);
    clrtoeol();

    refresh();
}

/* ── run a child process (endwin first, restore after) ──────────────────── */

static void run_child(const char *cmd, char *const argv[])
{
    endwin();

    pid_t pid = fork();
    if (pid < 0) {
        fprintf(stderr, "fork: %s\n", strerror(errno));
        return;
    }
    if (pid == 0) {
        execvp(cmd, argv);
        fprintf(stderr, "exec %s: %s\n", cmd, strerror(errno));
        _exit(127);
    }

    int status;
    while (waitpid(pid, &status, 0) < 0 && errno == EINTR)
        ;

    /* Restore ncurses */
    refresh();
}

/* ── delete a session's three files ─────────────────────────────────────── */

static void delete_session(const session_t *s)
{
    char path[1024];

    snprintf(path, sizeof(path), "%s/%s.ttyrec",      s->dir, s->sid);
    (void)unlink(path);

    snprintf(path, sizeof(path), "%s/%s.meta.json",   s->dir, s->sid);
    (void)unlink(path);

    snprintf(path, sizeof(path), "%s/%s.events.jsonl", s->dir, s->sid);
    (void)unlink(path);
}

/* ── ask a confirm question at the bottom of the screen ─────────────────── */

static int confirm_prompt(const char *msg)
{
    int rows, cols;
    getmaxyx(stdscr, rows, cols);

    attron(A_BOLD | COLOR_PAIR(2));
    mvprintw(rows - 1, 0, "%-*s", cols, msg);
    attroff(A_BOLD | COLOR_PAIR(2));
    clrtoeol();
    refresh();

    int ch = getch();
    return (ch == 'y' || ch == 'Y');
}

/* ── show a brief help overlay ───────────────────────────────────────────── */

static void show_help(void)
{
    int rows, cols;
    getmaxyx(stdscr, rows, cols);

    int bh = 14, bw = 52;
    int by = (rows - bh) / 2;
    int bx = (cols - bw) / 2;
    if (by < 0) by = 0;
    if (bx < 0) bx = 0;

    attron(A_REVERSE);
    for (int r = by; r < by + bh && r < rows; r++) {
        mvprintw(r, bx, "%-*s", bw, "");
    }
    attroff(A_REVERSE);

    const char *lines[] = {
        " pmp-rec TUI — key bindings",
        " ──────────────────────────────────────────────── ",
        " ↑ / k          move up",
        " ↓ / j          move down",
        " PgUp           scroll up 10 rows",
        " PgDn           scroll down 10 rows",
        " g / Home       go to top",
        " G / End        go to bottom",
        " p / Enter      play selected session",
        " t              tail (follow) selected session",
        " d              delete session (with confirm)",
        " r              refresh session list",
        " q / Q          quit",
        " [any key]      close this help",
    };
    int nlines = (int)(sizeof(lines) / sizeof(lines[0]));

    attron(A_BOLD);
    for (int i = 0; i < nlines && (by + i) < rows; i++) {
        mvprintw(by + i, bx, "%-*s", bw, lines[i]);
    }
    attroff(A_BOLD);

    refresh();
    getch();
}

/* ── entry point ─────────────────────────────────────────────────────────── */

int cmd_tui(int argc, char *argv[])
{
    const char *storage_dir = "/var/lib/pmp-rec";

    for (int i = 0; i < argc - 1; i++) {
        if (strcmp(argv[i], "--dir") == 0)
            storage_dir = argv[i + 1];
    }

    /* ── load initial session list ── */
    session_t *sessions = NULL;
    int        count    = 0;

    if (load_sessions(storage_dir, &sessions, &count) != 0) {
        fprintf(stderr, "Cannot open storage dir: %s: %s\n",
                storage_dir, strerror(errno));
        return 1;
    }

    /* ── init ncurses ── */
    initscr();
    start_color();
    use_default_colors();
    noecho();
    cbreak();
    keypad(stdscr, TRUE);
    curs_set(0);

    /* Color pair 1: green on default — ACTIVE sessions */
    init_pair(1, COLOR_GREEN,  -1);
    /* Color pair 2: red on default — confirm/error prompts */
    init_pair(2, COLOR_RED,    -1);

    signal(SIGWINCH, handle_sigwinch);

    int selected   = 0;
    int scroll_off = 0;

    draw_screen(sessions, count, selected, scroll_off, storage_dir);

    for (;;) {
        /* Handle terminal resize */
        if (g_resized) {
            g_resized = 0;
            endwin();
            refresh();
        }

        int rows, cols;
        getmaxyx(stdscr, rows, cols);
        (void)cols;
        int list_rows = rows - 5;
        if (list_rows < 1) list_rows = 1;

        int ch = getch();

        switch (ch) {
        /* ── navigation ── */
        case 'k':
        case KEY_UP:
            if (selected > 0) {
                selected--;
                if (selected < scroll_off)
                    scroll_off = selected;
            }
            break;

        case 'j':
        case KEY_DOWN:
            if (selected < count - 1) {
                selected++;
                if (selected >= scroll_off + list_rows)
                    scroll_off = selected - list_rows + 1;
            }
            break;

        case KEY_PPAGE: /* Page Up */
            selected -= 10;
            if (selected < 0) selected = 0;
            if (selected < scroll_off)
                scroll_off = selected;
            break;

        case KEY_NPAGE: /* Page Down */
            selected += 10;
            if (selected >= count) selected = count > 0 ? count - 1 : 0;
            if (selected >= scroll_off + list_rows)
                scroll_off = selected - list_rows + 1;
            if (scroll_off < 0) scroll_off = 0;
            break;

        case 'g':
        case KEY_HOME:
            selected   = 0;
            scroll_off = 0;
            break;

        case 'G':
        case KEY_END:
            selected = count > 0 ? count - 1 : 0;
            scroll_off = selected - list_rows + 1;
            if (scroll_off < 0) scroll_off = 0;
            break;

        /* ── play ── */
        case 'p':
        case '\n':
        case KEY_ENTER:
            if (count > 0 && selected < count) {
                char *cmd_argv[] = {
                    "pmp-rec-cli", "play",
                    "--dir", (char *)storage_dir,
                    sessions[selected].sid,
                    NULL
                };
                run_child("pmp-rec-cli", cmd_argv);
            }
            break;

        /* ── tail ── */
        case 't':
            if (count > 0 && selected < count) {
                char *cmd_argv[] = {
                    "pmp-rec-cli", "tail",
                    "--dir", (char *)storage_dir,
                    sessions[selected].sid,
                    NULL
                };
                run_child("pmp-rec-cli", cmd_argv);
            }
            break;

        /* ── delete ── */
        case 'd':
            if (count > 0 && selected < count) {
                char prompt[128];
                snprintf(prompt, sizeof(prompt),
                         " Delete %s? [y/N]: ", sessions[selected].sid);

                if (confirm_prompt(prompt)) {
                    delete_session(&sessions[selected]);

                    /* Reload list */
                    free_sessions(sessions);
                    sessions = NULL;
                    count    = 0;
                    load_sessions(storage_dir, &sessions, &count);

                    if (selected >= count)
                        selected = count > 0 ? count - 1 : 0;
                    if (scroll_off > selected)
                        scroll_off = selected;
                }
            }
            break;

        /* ── refresh ── */
        case 'r':
            free_sessions(sessions);
            sessions = NULL;
            count    = 0;
            load_sessions(storage_dir, &sessions, &count);
            if (selected >= count)
                selected = count > 0 ? count - 1 : 0;
            if (scroll_off > selected)
                scroll_off = selected;
            break;

        /* ── help ── */
        case '?':
            show_help();
            break;

        /* ── quit ── */
        case 'q':
        case 'Q':
            goto done;

        default:
            break;
        }

        /* Clamp scroll_off */
        if (scroll_off < 0) scroll_off = 0;
        if (count > 0 && scroll_off > count - 1)
            scroll_off = count - 1;

        draw_screen(sessions, count, selected, scroll_off, storage_dir);
    }

done:
    endwin();
    free_sessions(sessions);
    return 0;
}
