#define _GNU_SOURCE
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <errno.h>
#include <endian.h>
#include "pmp_proto.h"

uint64_t pmp_clock_ns(void)
{
    struct timespec ts;
    clock_gettime(CLOCK_REALTIME, &ts);
    return (uint64_t)ts.tv_sec * 1000000000ULL + (uint64_t)ts.tv_nsec;
}

void frame_parser_init(struct frame_parser *p)
{
    memset(p, 0, sizeof(*p));
}

void frame_parser_reset(struct frame_parser *p)
{
    free(p->payload_buf);
    p->payload_buf = NULL;
    p->payload_pos = 0;
    p->hdr_pos     = 0;
    p->have_header = 0;
}

void frame_parser_free(struct frame_parser *p)
{
    free(p->payload_buf);
    p->payload_buf = NULL;
}

int frame_parser_feed(struct frame_parser *p,
                      const uint8_t *buf, size_t len,
                      size_t *consumed, struct pmp_frame *out)
{
    size_t pos = 0;

    *consumed = 0;

    while (pos < len) {
        if (!p->have_header) {
            size_t need = sizeof(struct pmp_frame_hdr) - p->hdr_pos;
            size_t take = len - pos;
            if (take > need) take = need;

            memcpy(p->hdr_buf + p->hdr_pos, buf + pos, take);
            p->hdr_pos += take;
            pos        += take;

            if (p->hdr_pos < sizeof(struct pmp_frame_hdr))
                continue;

            memcpy(&p->hdr, p->hdr_buf, sizeof(struct pmp_frame_hdr));
            p->hdr.magic       = le32toh(p->hdr.magic);
            p->hdr.version     = le16toh(p->hdr.version);
            p->hdr.type        = le16toh(p->hdr.type);
            p->hdr.ts_ns       = le64toh(p->hdr.ts_ns);
            p->hdr.seq         = le64toh(p->hdr.seq);
            p->hdr.payload_len = le32toh(p->hdr.payload_len);

            if (p->hdr.payload_len > PMP_MAX_PAYLOAD)
                return -EMSGSIZE;

            if (p->hdr.payload_len > 0) {
                p->payload_buf = malloc(p->hdr.payload_len);
                if (!p->payload_buf)
                    return -ENOMEM;
            }
            p->payload_pos = 0;
            p->have_header = 1;
        }

        if (p->hdr.payload_len > 0) {
            size_t need = p->hdr.payload_len - p->payload_pos;
            size_t take = len - pos;
            if (take > need) take = need;

            memcpy(p->payload_buf + p->payload_pos, buf + pos, take);
            p->payload_pos += take;
            pos            += take;

            if (p->payload_pos < p->hdr.payload_len)
                continue;
        }

        out->hdr     = p->hdr;
        out->payload = p->payload_buf;
        *consumed    = pos;

        /* Reset for next frame — caller must free payload_buf via frame_parser_reset */
        p->hdr_pos     = 0;
        p->have_header = 0;
        p->payload_buf = NULL;
        p->payload_pos = 0;

        return 1; /* complete frame */
    }

    *consumed = pos;
    return 0;
}

ssize_t frame_encode(uint8_t *buf, size_t bufsz,
                     uint16_t type, uint64_t ts_ns, uint64_t seq,
                     const void *payload, uint32_t payload_len,
                     int include_magic)
{
    struct pmp_frame_hdr h;
    size_t total = sizeof(h) + payload_len;

    if (bufsz < total)
        return -ENOBUFS;

    h.magic       = htole32(include_magic ? PMP_PROTO_MAGIC : 0u);
    h.version     = htole16(PMP_PROTO_VERSION);
    h.type        = htole16(type);
    h.ts_ns       = htole64(ts_ns);
    h.seq         = htole64(seq);
    h.payload_len = htole32(payload_len);

    memcpy(buf, &h, sizeof(h));
    if (payload_len > 0 && payload)
        memcpy(buf + sizeof(h), payload, payload_len);

    return (ssize_t)total;
}
