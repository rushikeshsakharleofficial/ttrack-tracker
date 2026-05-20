#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <pwd.h>
#include <errno.h>
#include "pmp_paths.h"
#include "pmp_log.h"

/* Returns malloc'd path to user's real shell, validated against shells.allow.
 * Falls back to /bin/sh if validation fails.
 * Caller must free().
 */
char *pmp_resolve_shell(void)
{
    struct passwd *pw;
    FILE *f;
    char line[512];
    const char *shell = NULL;
    char *result = NULL;
    int found = 0;

    pw = getpwuid(getuid());
    if (pw && pw->pw_shell && pw->pw_shell[0] != '\0')
        shell = pw->pw_shell;
    else
        shell = "/bin/sh";

    /* If it points back to the shim, force fallback */
    if (strstr(shell, "pmp-rec"))
        shell = "/bin/sh";

    f = fopen(PMP_CONF_SHELLS, "r");
    if (f) {
        while (fgets(line, sizeof(line), f)) {
            size_t n = strlen(line);
            while (n > 0 && (line[n-1] == '\n' || line[n-1] == '\r' || line[n-1] == ' '))
                line[--n] = '\0';
            if (n == 0 || line[0] == '#') continue;
            if (strcmp(line, shell) == 0) {
                found = 1;
                break;
            }
        }
        fclose(f);

        if (!found) {
            PMP_LOG_WARN("shell %s not in %s, falling back to /bin/sh",
                         shell, PMP_CONF_SHELLS);
            shell = "/bin/sh";
        }
    }
    /* If shells.allow missing, trust pw_shell */

    result = strdup(shell);
    if (!result) return strdup("/bin/sh");
    return result;
}

/* Build argv for the user shell.
 * If explicit_argv is non-NULL, use it directly.
 * Otherwise build [shell, "-l"] for login or [shell] for plain.
 */
char **pmp_build_shell_argv(const char *shell, char *const *explicit_argv, int login)
{
    char **argv;

    if (explicit_argv && explicit_argv[0]) {
        /* Count args */
        int n = 0;
        while (explicit_argv[n]) n++;
        argv = calloc((size_t)(n + 1), sizeof(char *));
        if (!argv) return NULL;
        for (int i = 0; i < n; i++)
            argv[i] = explicit_argv[i];
        argv[n] = NULL;
        return argv;
    }

    argv = calloc(3, sizeof(char *));
    if (!argv) return NULL;
    argv[0] = (char *)shell;
    if (login)
        argv[1] = "-l";
    return argv;
}
