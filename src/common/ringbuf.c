#define _GNU_SOURCE
#include <stdlib.h>
#include <string.h>
#include <errno.h>
#include <stdint.h>

struct ringbuf {
    uint8_t *buf;
    size_t   cap;
    size_t   head;   /* write position */
    size_t   tail;   /* read position */
    size_t   used;
};

struct ringbuf *ringbuf_alloc(size_t cap)
{
    struct ringbuf *rb = calloc(1, sizeof(*rb));
    if (!rb) return NULL;
    rb->buf = malloc(cap);
    if (!rb->buf) { free(rb); return NULL; }
    rb->cap = cap;
    return rb;
}

void ringbuf_free(struct ringbuf *rb)
{
    if (rb) { free(rb->buf); free(rb); }
}

size_t ringbuf_avail(const struct ringbuf *rb)
{
    return rb->cap - rb->used;
}

size_t ringbuf_used(const struct ringbuf *rb)
{
    return rb->used;
}

/* Returns bytes written; may be less than len if full (drops remaining). */
size_t ringbuf_write(struct ringbuf *rb, const uint8_t *data, size_t len)
{
    size_t avail = ringbuf_avail(rb);
    size_t n = len < avail ? len : avail;
    size_t first = rb->cap - rb->head;

    if (n == 0) return 0;

    if (n <= first) {
        memcpy(rb->buf + rb->head, data, n);
    } else {
        memcpy(rb->buf + rb->head, data, first);
        memcpy(rb->buf, data + first, n - first);
    }
    rb->head = (rb->head + n) % rb->cap;
    rb->used += n;
    return n;
}

/* Peek at up to len contiguous bytes from tail. Returns count. */
size_t ringbuf_peek(const struct ringbuf *rb, uint8_t *out, size_t len)
{
    size_t n = rb->used < len ? rb->used : len;
    size_t first = rb->cap - rb->tail;

    if (n == 0) return 0;
    if (n <= first) {
        memcpy(out, rb->buf + rb->tail, n);
    } else {
        memcpy(out, rb->buf + rb->tail, first);
        memcpy(out + first, rb->buf, n - first);
    }
    return n;
}

void ringbuf_consume(struct ringbuf *rb, size_t n)
{
    if (n > rb->used) n = rb->used;
    rb->tail = (rb->tail + n) % rb->cap;
    rb->used -= n;
}
