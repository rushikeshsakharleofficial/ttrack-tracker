#ifndef PMP_COMPAT_H
#define PMP_COMPAT_H

#include <sys/types.h>
#include <stdint.h>

/* loginuid sentinel: unset on older kernels / non-PAM contexts */
#define PMP_LOGINUID_UNSET  ((uint32_t)4294967295u)   /* 0xFFFFFFFF */

/* Portable MIN/MAX without double-evaluation */
#ifndef MIN
#  define MIN(a,b) ((a) < (b) ? (a) : (b))
#endif
#ifndef MAX
#  define MAX(a,b) ((a) > (b) ? (a) : (b))
#endif

/* Silence unused-parameter warnings */
#define PMP_UNUSED(x) ((void)(x))

/* GCC/Clang branch prediction hints */
#define PMP_LIKELY(x)   __builtin_expect(!!(x), 1)
#define PMP_UNLIKELY(x) __builtin_expect(!!(x), 0)

#endif /* PMP_COMPAT_H */
