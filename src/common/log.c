#define _GNU_SOURCE
#include <stdarg.h>
#include <stdio.h>
#include <syslog.h>
#include "trackterm_log.h"

int trackterm_log_stderr = 0;
int trackterm_log_level  = LOG_INFO;

void trackterm_log_init(const char *ident, int use_stderr, int level)
{
    trackterm_log_stderr = use_stderr;
    trackterm_log_level  = level;
    openlog(ident, LOG_PID | LOG_NDELAY, LOG_AUTH);
}

void trackterm_log_msg(int priority, const char *fmt, ...)
{
    va_list ap;

    if (priority > trackterm_log_level)
        return;

    va_start(ap, fmt);
    if (trackterm_log_stderr) {
        vfprintf(stderr, fmt, ap);
        fputc('\n', stderr);
    } else {
        vsyslog(priority, fmt, ap);
    }
    va_end(ap);
}
