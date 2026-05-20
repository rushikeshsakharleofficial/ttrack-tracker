#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <assert.h>
#include <fcntl.h>
#include <unistd.h>
#include <endian.h>
#include "pmp_ttyrec.h"

static int tests_run = 0;
static int tests_ok  = 0;

#define CHECK(cond, msg) do { \
    tests_run++; \
    if (cond) { tests_ok++; printf("OK  %s\n", msg); } \
    else { printf("FAIL %s (line %d)\n", msg, __LINE__); } \
} while(0)

static void test_header_layout(void)
{
    CHECK(sizeof(struct ttyrec_hdr) == 12, "ttyrec_hdr is 12 bytes");

    struct ttyrec_hdr h;
    h.tv_sec  = 0x11223344;
    h.tv_usec = 0xAABBCCDD;
    h.len     = 0x12345678;

    uint8_t *b = (uint8_t *)&h;
    /* Little-endian layout check */
    CHECK(b[0] == 0x44, "tv_sec LE byte 0");
    CHECK(b[1] == 0x33, "tv_sec LE byte 1");
    CHECK(b[4] == 0xDD, "tv_usec LE byte 0");
    CHECK(b[8] == 0x78, "len LE byte 0");
}

static void test_write_read_frame(void)
{
    char tmpfile[] = "/tmp/pmp_ttyrec_test_XXXXXX";
    int fd = mkstemp(tmpfile);
    if (fd < 0) { CHECK(0, "mkstemp"); return; }
    unlink(tmpfile);

    const char *data = "test frame data";
    uint32_t dlen = (uint32_t)strlen(data);

    int r = ttyrec_write_frame(fd, data, dlen);
    CHECK(r == 0, "write_frame returns 0");

    lseek(fd, 0, SEEK_SET);

    struct ttyrec_hdr h;
    ssize_t n = read(fd, &h, sizeof(h));
    CHECK(n == sizeof(h), "read header");
    CHECK(le32toh(h.len) == dlen, "frame len matches");

    char rbuf[64];
    n = read(fd, rbuf, dlen);
    CHECK(n == (ssize_t)dlen, "read payload");
    CHECK(memcmp(rbuf, data, dlen) == 0, "payload matches");

    close(fd);
}

int main(void)
{
    test_header_layout();
    test_write_read_frame();

    printf("\n%d/%d tests passed\n", tests_ok, tests_run);
    return (tests_ok == tests_run) ? 0 : 1;
}
