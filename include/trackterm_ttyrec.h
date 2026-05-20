#ifndef TRACKTERM_TTYREC_H
#define TRACKTERM_TTYREC_H

#include <stdint.h>
#include <stddef.h>
#include <sys/types.h>

/*
 * ttyrec frame header — 12 bytes, all little-endian uint32.
 * Compatible with ttyplay, ipbt, termrec.
 */
struct ttyrec_hdr {
    uint32_t tv_sec;   /* seconds since epoch (Y2038-limited, by spec) */
    uint32_t tv_usec;  /* microseconds */
    uint32_t len;      /* payload length in bytes */
} __attribute__((packed));

/* Write one ttyrec frame to fd. Returns 0 on success, -1 on error. */
int ttyrec_write_frame(int fd, const void *data, uint32_t len);

/* Write events.jsonl line to fd. */
int ttyrec_write_event(int fd, double t_sec, const char *json_body);

#endif /* TRACKTERM_TTYREC_H */
