#define _GNU_SOURCE
#define PAM_SM_SESSION

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <syslog.h>

#include <security/pam_modules.h>
#include <security/pam_ext.h>

/* Forward declarations from companion files */
int  pmp_pam_chain_env(pam_handle_t *pamh);
char *pmp_pam_get_sid(pam_handle_t *pamh);
int  pmp_pam_write_marker(pam_handle_t *pamh, const char *sid);
void pmp_pam_remove_marker(const char *sid);

/*
 * Skip recording for these services / users (compiled-in defaults;
 * config file can extend via /etc/pmp-rec/recd.conf exclude_services,
 * exclude_users — not parsed here to keep the module lean).
 */
static const char *skip_services[] = {
    "crond", "cron", "sshd-keys", "polkit-1", "gdm", NULL
};

static const char *skip_users[] = {
    "root",   /* can be overridden in conf */
    NULL
};

static int should_skip(pam_handle_t *pamh, const char *service)
{
    const char *user = NULL;

    /* Check service list */
    for (int i = 0; skip_services[i]; i++)
        if (service && strcmp(service, skip_services[i]) == 0)
            return 1;

    /* Check user list */
    pam_get_item(pamh, PAM_USER, (const void **)&user);
    for (int i = 0; skip_users[i]; i++)
        if (user && strcmp(user, skip_users[i]) == 0)
            return 1; /* skip — user in exclusion list */

    return 0;
}

PAM_EXTERN int pam_sm_open_session(pam_handle_t *pamh, int flags,
                                   int argc, const char **argv)
{
    const char *service = NULL;
    int r;

    (void)flags; (void)argc; (void)argv;

    pam_get_item(pamh, PAM_SERVICE, (const void **)&service);

    if (should_skip(pamh, service))
        return PAM_SUCCESS;

    /* Propagate service name into PAM env so shim can read it.
     * PAM_SERVICE is not always visible in child process environment. */
    if (service) {
        char envbuf[64];
        snprintf(envbuf, sizeof(envbuf), "PMP_REC_SERVICE=%s", service);
        pam_putenv(pamh, envbuf);
    }

    r = pmp_pam_chain_env(pamh);
    if (r != PAM_SUCCESS)
        return PAM_SUCCESS; /* fail-open */

    char *sid = pmp_pam_get_sid(pamh);
    if (sid) {
        pmp_pam_write_marker(pamh, sid);
        free(sid);
    }

    return PAM_SUCCESS;
}

PAM_EXTERN int pam_sm_close_session(pam_handle_t *pamh, int flags,
                                    int argc, const char **argv)
{
    const char *sid_env;

    (void)flags; (void)argc; (void)argv;

    sid_env = pam_getenv(pamh, "PMP_REC_SID");
    if (sid_env && sid_env[0])
        pmp_pam_remove_marker(sid_env);

    return PAM_SUCCESS;
}

/* Stubs for unused PAM interfaces required by the ABI */
PAM_EXTERN int pam_sm_authenticate(pam_handle_t *pamh, int flags,
                                   int argc, const char **argv)
{ (void)pamh; (void)flags; (void)argc; (void)argv; return PAM_IGNORE; }

PAM_EXTERN int pam_sm_setcred(pam_handle_t *pamh, int flags,
                              int argc, const char **argv)
{ (void)pamh; (void)flags; (void)argc; (void)argv; return PAM_IGNORE; }
