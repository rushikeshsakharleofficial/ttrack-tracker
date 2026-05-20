#ifndef TRACKTERM_UUID_H
#define TRACKTERM_UUID_H

/* Fills buf with a lowercase UUID v4 string (36 chars + NUL). */
int trackterm_uuid_generate(char buf[37]);

#endif /* TRACKTERM_UUID_H */
