#define _GNU_SOURCE
#include <stdarg.h>
#include <stdio.h>
#include <syslog.h>
#include "pmp_log.h"

int pmp_log_stderr = 0;
int pmp_log_level  = LOG_INFO;

void pmp_log_init(const char *ident, int use_stderr, int level)
{
    pmp_log_stderr = use_stderr;
    pmp_log_level  = level;
    openlog(ident, LOG_PID | LOG_NDELAY, LOG_AUTH);
}

void pmp_log_msg(int priority, const char *fmt, ...)
{
    va_list ap;

    if (priority > pmp_log_level)
        return;

    va_start(ap, fmt);
    if (pmp_log_stderr) {
        vfprintf(stderr, fmt, ap);
        fputc('\n', stderr);
    } else {
        vsyslog(priority, fmt, ap);
    }
    va_end(ap);
}
