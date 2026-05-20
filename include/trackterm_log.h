#ifndef TRACKTERM_LOG_H
#define TRACKTERM_LOG_H

#include <syslog.h>
#include <stdio.h>

extern int trackterm_log_stderr;
extern int trackterm_log_level;

void trackterm_log_init(const char *ident, int use_stderr, int level);
void trackterm_log_msg(int priority, const char *fmt, ...)
    __attribute__((format(printf, 2, 3)));

#define TRACKTERM_LOG_ERR(fmt, ...)  trackterm_log_msg(LOG_ERR,     fmt, ##__VA_ARGS__)
#define TRACKTERM_LOG_WARN(fmt, ...) trackterm_log_msg(LOG_WARNING,  fmt, ##__VA_ARGS__)
#define TRACKTERM_LOG_INFO(fmt, ...) trackterm_log_msg(LOG_INFO,     fmt, ##__VA_ARGS__)
#define TRACKTERM_LOG_DBG(fmt, ...)  trackterm_log_msg(LOG_DEBUG,    fmt, ##__VA_ARGS__)

#endif /* TRACKTERM_LOG_H */
