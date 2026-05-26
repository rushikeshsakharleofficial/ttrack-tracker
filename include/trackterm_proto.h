#ifndef TRACKTERM_PROTO_H
#define TRACKTERM_PROTO_H

#include <stdint.h>
#include <sys/ioctl.h>

#define TRACKTERM_PROTO_MAGIC   0x504D5052u   /* "PMPR" little-endian */
#define TRACKTERM_PROTO_VERSION 1
#define TRACKTERM_MAX_PAYLOAD   (64u * 1024u)

enum trackterm_frame_type {
    TRACKTERM_F_HELLO     = 1,
    TRACKTERM_F_OUT       = 2,
    TRACKTERM_F_RESIZE    = 3,
    TRACKTERM_F_CLOSE     = 4,
    TRACKTERM_F_HEARTBEAT = 5,
    TRACKTERM_F_GAP       = 6,  /* reconnect gap: payload is struct trackterm_gap */
};

/* All multi-byte fields are little-endian. 28 bytes, no padding. */
struct trackterm_frame_hdr {
    uint32_t magic;        /* TRACKTERM_PROTO_MAGIC — first frame per connection */
    uint16_t version;
    uint16_t type;         /* enum trackterm_frame_type */
    uint64_t ts_ns;        /* CLOCK_REALTIME nanoseconds since epoch */
    uint64_t seq;          /* monotonic, per-session */
    uint32_t payload_len;
} __attribute__((packed));

/* TRACKTERM_F_HELLO payload */
struct trackterm_hello {
    char     sid[37];          /* UUIDv4, NUL-terminated */
    char     parent_sid[37];   /* empty string if no parent */
    uint32_t loginuid;         /* from /proc/self/loginuid */
    uint32_t euid;
    uint32_t gid;
    uint16_t rows;
    uint16_t cols;
    uint16_t xpixel;
    uint16_t ypixel;
    char     service[32];      /* PAM service name */
    char     tty[64];          /* /dev/pts/N */
    char     term[32];         /* $TERM */
    char     rhost[64];        /* SSH client IP or empty */
    char     ruser[32];        /* real (login) username */
    char     hostname[64];     /* gethostname() */
} __attribute__((packed));

/* TRACKTERM_F_RESIZE payload: mirrors struct winsize (sys/ioctl.h) */
struct trackterm_resize {
    uint16_t rows;
    uint16_t cols;
    uint16_t xpixel;
    uint16_t ypixel;
} __attribute__((packed));

/* TRACKTERM_F_CLOSE payload */
struct trackterm_close {
    int32_t exit_status;   /* child exit status, LE */
} __attribute__((packed));

/* TRACKTERM_F_GAP payload: sent on reconnect to record missing interval */
struct trackterm_gap {
    double gap_seconds;        /* wall-clock seconds daemon was unreachable */
} __attribute__((packed));

/* TRACKTERM_F_OUT: raw bytes follow the header — no separate struct */

/* --- Frame encoder/decoder --- */

struct trackterm_frame {
    struct trackterm_frame_hdr hdr;
    const void          *payload;
};

/* Incremental parser for stream-oriented sockets */
struct frame_parser {
    struct trackterm_frame_hdr hdr;
    uint8_t  hdr_buf[sizeof(struct trackterm_frame_hdr)];
    size_t   hdr_pos;       /* bytes received so far for header */
    uint8_t *payload_buf;   /* malloc'd, len = hdr.payload_len */
    size_t   payload_pos;
    int      have_header;
};

void frame_parser_init(struct frame_parser *p);
void frame_parser_reset(struct frame_parser *p);
void frame_parser_free(struct frame_parser *p);

/*
 * Feed data into the parser. Returns:
 *   > 0  : complete frame ready, consume *consumed bytes from buf
 *   = 0  : need more data, *consumed == len
 *   < 0  : parse error
 */
int frame_parser_feed(struct frame_parser *p,
                      const uint8_t *buf, size_t len,
                      size_t *consumed, struct trackterm_frame *out);

/*
 * Encode a frame into buf (must be >= sizeof(hdr) + payload_len).
 * Returns total bytes written or -1 on error.
 */
ssize_t frame_encode(uint8_t *buf, size_t bufsz,
                     uint16_t type, uint64_t ts_ns, uint64_t seq,
                     const void *payload, uint32_t payload_len,
                     int include_magic);

uint64_t trackterm_clock_ns(void);

#endif /* TRACKTERM_PROTO_H */
