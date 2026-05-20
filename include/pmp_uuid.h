#ifndef PMP_UUID_H
#define PMP_UUID_H

/* Fills buf with a lowercase UUID v4 string (36 chars + NUL). */
int pmp_uuid_generate(char buf[37]);

#endif /* PMP_UUID_H */
