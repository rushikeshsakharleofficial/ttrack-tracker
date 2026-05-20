#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <assert.h>
#include "pmp_proto.h"

static int tests_run = 0;
static int tests_ok  = 0;

#define CHECK(cond, msg) do { \
    tests_run++; \
    if (cond) { tests_ok++; printf("OK  %s\n", msg); } \
    else { printf("FAIL %s (line %d)\n", msg, __LINE__); } \
} while(0)

static void test_encode_decode(void)
{
    uint8_t buf[256];
    const char *payload = "hello world";
    uint32_t plen = (uint32_t)strlen(payload);

    ssize_t n = frame_encode(buf, sizeof(buf),
                             PMP_F_OUT, 123456789ULL, 42,
                             payload, plen, 1);
    CHECK(n == (ssize_t)(sizeof(struct pmp_frame_hdr) + plen),
          "encode returns correct length");

    struct frame_parser p;
    frame_parser_init(&p);

    struct pmp_frame f;
    size_t consumed;
    int r = frame_parser_feed(&p, buf, (size_t)n, &consumed, &f);

    CHECK(r == 1, "frame_parser_feed: complete frame");
    CHECK(consumed == (size_t)n, "consumed == n");
    CHECK(f.hdr.type == PMP_F_OUT, "type == PMP_F_OUT");
    CHECK(f.hdr.seq == 42, "seq == 42");
    CHECK(f.hdr.payload_len == plen, "payload_len correct");
    CHECK(memcmp(f.payload, payload, plen) == 0, "payload matches");

    free((void *)f.payload);
    frame_parser_free(&p);
}

static void test_partial_feed(void)
{
    uint8_t buf[128];
    const char *payload = "partial test";
    uint32_t plen = (uint32_t)strlen(payload);

    ssize_t total = frame_encode(buf, sizeof(buf),
                                 PMP_F_HEARTBEAT, 0, 1,
                                 payload, plen, 0);

    struct frame_parser p;
    frame_parser_init(&p);
    struct pmp_frame f;
    size_t consumed;

    /* Feed byte-by-byte */
    int got = 0;
    for (size_t i = 0; i < (size_t)total; i++) {
        int r = frame_parser_feed(&p, buf + i, 1, &consumed, &f);
        if (r == 1) { got = 1; free((void *)f.payload); break; }
    }
    CHECK(got == 1, "partial feed: frame reconstructed");
    frame_parser_free(&p);
}

static void test_clock_ns(void)
{
    uint64_t a = pmp_clock_ns();
    uint64_t b = pmp_clock_ns();
    CHECK(b >= a, "clock_ns monotone");
    CHECK(a > 0, "clock_ns > 0");
}

int main(void)
{
    test_encode_decode();
    test_partial_feed();
    test_clock_ns();

    printf("\n%d/%d tests passed\n", tests_ok, tests_run);
    return (tests_ok == tests_run) ? 0 : 1;
}
