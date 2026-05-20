#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <syslog.h>

#include <security/pam_modules.h>
#include <security/pam_ext.h>

/* UUID generation using /proc/sys/kernel/random/uuid */
static int mint_uuid(char out[37])
{
    FILE *f = fopen("/proc/sys/kernel/random/uuid", "r");
    if (!f) return -1;
    int n = fread(out, 1, 36, f);
    fclose(f);
    if (n < 36) return -1;
    out[36] = '\0';
    if (out[35] == '\n') out[35] = '\0';
    return 0;
}

/*
 * Chain PAM env for session SIDs:
 *
 * Before:  PMP_REC_SID=A  (from parent session)
 * After:   PMP_REC_SID=B  (new session)
 *          PMP_REC_PARENT=A
 *          PMP_REC_ACTIVE=  (cleared so shim re-engages)
 *
 * If no existing SID: just mint new SID, leave PARENT empty.
 */
int pmp_pam_chain_env(pam_handle_t *pamh)
{
    char new_sid[37];
    const char *old_sid;

    /* Read existing SID from PAM environment */
    old_sid = pam_getenv(pamh, "PMP_REC_SID");

    if (mint_uuid(new_sid) < 0) {
        pam_syslog(pamh, LOG_ERR, "pmp: mint_uuid failed");
        return PAM_SESSION_ERR;
    }

    /* Set new SID */
    char buf[64];
    snprintf(buf, sizeof(buf), "PMP_REC_SID=%s", new_sid);
    pam_putenv(pamh, buf);

    /* Promote old SID to parent */
    if (old_sid && old_sid[0]) {
        snprintf(buf, sizeof(buf), "PMP_REC_PARENT=%s", old_sid);
    } else {
        strcpy(buf, "PMP_REC_PARENT=");
    }
    pam_putenv(pamh, buf);

    /* Clear active flag so profile.d re-engages for this new session */
    pam_putenv(pamh, "PMP_REC_ACTIVE=");
    pam_putenv(pamh, "PMP_REC_SHIM_CHILD=");

    pam_syslog(pamh, LOG_INFO,
               "pmp: new session SID=%s parent=%s",
               new_sid, old_sid ? old_sid : "(none)");

    return PAM_SUCCESS;
}

char *pmp_pam_get_sid(pam_handle_t *pamh)
{
    const char *s = pam_getenv(pamh, "PMP_REC_SID");
    return s ? strdup(s) : NULL;
}
