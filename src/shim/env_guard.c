#define _GNU_SOURCE
#include <stdlib.h>
#include <string.h>
#include <stdio.h>
#include <unistd.h>
#include <stdint.h>
#include "trackterm_log.h"
#include "trackterm_paths.h"

/* Returns 1 if this process is already inside a trackterm-rec session.
 * Checked before allocating PTY — prevents double-wrapping.
 */
int trackterm_already_recording(void)
{
    const char *active = getenv("TRACKTERM_REC_ACTIVE");
    return (active && active[0] != '\0');
}

/* Stamp env for the child shell:
 * - TRACKTERM_REC_ACTIVE=1  (re-entrancy guard for profile.d)
 * - TRACKTERM_REC_SHIM_CHILD=1  (guard for shim itself)
 * - TRACKTERM_REC_SID=<sid>
 * - TRACKTERM_REC_PARENT=<parent_sid or "">
 * - TRACKTERM_REC_LOGINUID=<uid>
 * - Clear SHELL so subshells don't re-enter shim via $SHELL
 */
void trackterm_env_stamp_child(const char *sid, const char *parent_sid,
                         uint32_t loginuid, const char *real_shell)
{
    char buf[64];

    setenv("TRACKTERM_REC_ACTIVE",      "1",    1);
    setenv("TRACKTERM_REC_SHIM_CHILD",  "1",    1);
    setenv("TRACKTERM_REC_SID",          sid,   1);
    setenv("TRACKTERM_REC_PARENT",       parent_sid ? parent_sid : "", 1);

    snprintf(buf, sizeof(buf), "%u", loginuid);
    setenv("TRACKTERM_REC_LOGINUID", buf, 1);

    /* Replace SHELL with real shell path so `sudo -s` doesn't re-invoke shim */
    if (real_shell)
        setenv("SHELL", real_shell, 1);
}

/* Read PAM-stamped SID from env (set by pam_record.so before exec).
 * Returns pointer into environ; NULL if not set.
 */
const char *trackterm_env_get_sid(void)
{
    return getenv("TRACKTERM_REC_SID");
}

const char *trackterm_env_get_parent_sid(void)
{
    return getenv("TRACKTERM_REC_PARENT");
}
