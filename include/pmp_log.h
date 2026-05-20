#ifndef PMP_LOG_H
#define PMP_LOG_H

#include <syslog.h>
#include <stdio.h>

extern int pmp_log_stderr;
extern int pmp_log_level;

void pmp_log_init(const char *ident, int use_stderr, int level);
void pmp_log_msg(int priority, const char *fmt, ...)
    __attribute__((format(printf, 2, 3)));

#define PMP_LOG_ERR(fmt, ...)  pmp_log_msg(LOG_ERR,     fmt, ##__VA_ARGS__)
#define PMP_LOG_WARN(fmt, ...) pmp_log_msg(LOG_WARNING,  fmt, ##__VA_ARGS__)
#define PMP_LOG_INFO(fmt, ...) pmp_log_msg(LOG_INFO,     fmt, ##__VA_ARGS__)
#define PMP_LOG_DBG(fmt, ...)  pmp_log_msg(LOG_DEBUG,    fmt, ##__VA_ARGS__)

#endif /* PMP_LOG_H */
