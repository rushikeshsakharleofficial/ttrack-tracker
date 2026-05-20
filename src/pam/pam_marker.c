#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <fcntl.h>
#include <sys/stat.h>
#include <errno.h>
#include <syslog.h>
#include <time.h>

#include <security/pam_modules.h>
#include <security/pam_ext.h>

#include "pmp_paths.h"

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

/*
 * Write /run/pmp-rec/sessions/<sid>.json so the daemon can cross-reference
 * PAM-stamped sessions against connected shims.
 */
int pmp_pam_write_marker(pam_handle_t *pamh, const char *sid)
{
    char path[512];
    char buf[1024];
    int fd, n;
    time_t now = time(NULL);
    const char *user, *rhost, *service, *tty;

    pam_get_item(pamh, PAM_USER,    (const void **)&user);
    pam_get_item(pamh, PAM_RHOST,   (const void **)&rhost);
    pam_get_item(pamh, PAM_SERVICE, (const void **)&service);
    pam_get_item(pamh, PAM_TTY,     (const void **)&tty);

    mkdir(PMP_RUN_DIR,      0755);
    mkdir(PMP_SESSIONS_DIR, 0755);

    snprintf(path, sizeof(path), "%s/%s.json", PMP_SESSIONS_DIR, sid);

    /* Escape PAM fields — PAM_RHOST is attacker-controlled (SSH client hostname) */
    char e_user[128], e_rhost[256], e_service[128], e_tty[256];
    json_escape(user    ? user    : "", e_user,    sizeof(e_user));
    json_escape(rhost   ? rhost   : "", e_rhost,   sizeof(e_rhost));
    json_escape(service ? service : "", e_service, sizeof(e_service));
    json_escape(tty     ? tty     : "", e_tty,     sizeof(e_tty));

    n = snprintf(buf, sizeof(buf),
                 "{\"sid\":\"%s\",\"user\":\"%s\","
                 "\"rhost\":\"%s\",\"service\":\"%s\","
                 "\"tty\":\"%s\",\"ts\":%ld}\n",
                 sid, e_user, e_rhost, e_service, e_tty,
                 (long)now);

    fd = open(path, O_WRONLY | O_CREAT | O_TRUNC | O_CLOEXEC, 0600);
    if (fd < 0) {
        pam_syslog(pamh, LOG_WARNING,
                   "pmp: cannot write marker %s: %s", path, strerror(errno));
        return -1;
    }
    write(fd, buf, (size_t)n);
    close(fd);
    return 0;
}

void pmp_pam_remove_marker(const char *sid)
{
    char path[512];
    snprintf(path, sizeof(path), "%s/%s.json", PMP_SESSIONS_DIR, sid);
    unlink(path);
}
