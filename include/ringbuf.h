#ifndef RINGBUF_H
#define RINGBUF_H

#include <stdint.h>
#include <stddef.h>

struct ringbuf;

struct ringbuf *ringbuf_alloc(size_t cap);
void            ringbuf_free(struct ringbuf *rb);
size_t          ringbuf_avail(const struct ringbuf *rb);
size_t          ringbuf_used(const struct ringbuf *rb);
size_t          ringbuf_write(struct ringbuf *rb, const uint8_t *data, size_t len);
size_t          ringbuf_peek(const struct ringbuf *rb, uint8_t *out, size_t len);
void            ringbuf_consume(struct ringbuf *rb, size_t n);

#endif /* RINGBUF_H */
