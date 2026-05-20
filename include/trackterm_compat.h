#ifndef TRACKTERM_COMPAT_H
#define TRACKTERM_COMPAT_H

#include <sys/types.h>
#include <stdint.h>

/* loginuid sentinel: unset on older kernels / non-PAM contexts */
#define TRACKTERM_LOGINUID_UNSET  ((uint32_t)4294967295u)   /* 0xFFFFFFFF */

/* Portable MIN/MAX without double-evaluation */
#ifndef MIN
#  define MIN(a,b) ((a) < (b) ? (a) : (b))
#endif
#ifndef MAX
#  define MAX(a,b) ((a) > (b) ? (a) : (b))
#endif

/* Silence unused-parameter warnings */
#define TRACKTERM_UNUSED(x) ((void)(x))

/* GCC/Clang branch prediction hints */
#define TRACKTERM_LIKELY(x)   __builtin_expect(!!(x), 1)
#define TRACKTERM_UNLIKELY(x) __builtin_expect(!!(x), 0)

#endif /* TRACKTERM_COMPAT_H */
