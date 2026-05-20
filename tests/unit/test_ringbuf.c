#define _GNU_SOURCE
#include <stdio.h>
#include <string.h>
#include <assert.h>
#include "ringbuf.h"

static int tests_run = 0;
static int tests_ok  = 0;

#define CHECK(cond, msg) do { \
    tests_run++; \
    if (cond) { tests_ok++; printf("OK  %s\n", msg); } \
    else { printf("FAIL %s (line %d)\n", msg, __LINE__); } \
} while(0)

int main(void)
{
    struct ringbuf *rb = ringbuf_alloc(16);
    CHECK(rb != NULL, "ringbuf_alloc");
    CHECK(ringbuf_avail(rb) == 16, "initial avail == cap");
    CHECK(ringbuf_used(rb) == 0, "initial used == 0");

    uint8_t data[] = "hello";
    size_t n = ringbuf_write(rb, data, 5);
    CHECK(n == 5, "write 5 bytes");
    CHECK(ringbuf_used(rb) == 5, "used == 5 after write");

    uint8_t out[16] = {0};
    size_t p = ringbuf_peek(rb, out, 5);
    CHECK(p == 5, "peek 5 bytes");
    CHECK(memcmp(out, "hello", 5) == 0, "peek data correct");

    ringbuf_consume(rb, 3);
    CHECK(ringbuf_used(rb) == 2, "used == 2 after consume 3");

    /* Wrap-around write */
    uint8_t big[14];
    memset(big, 'X', sizeof(big));
    n = ringbuf_write(rb, big, sizeof(big));
    CHECK(n == 14, "wrap-around write fills remaining");
    CHECK(ringbuf_avail(rb) == 0, "avail == 0 when full");

    /* Overflow: extra write drops */
    uint8_t extra[] = "overflow";
    n = ringbuf_write(rb, extra, sizeof(extra));
    CHECK(n == 0, "write on full ring returns 0");

    ringbuf_free(rb);
    CHECK(1, "ringbuf_free");

    printf("\n%d/%d tests passed\n", tests_ok, tests_run);
    return (tests_ok == tests_run) ? 0 : 1;
}
