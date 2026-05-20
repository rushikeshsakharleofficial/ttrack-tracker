#define _GNU_SOURCE
#include <stdio.h>
#include <string.h>
#include "trackterm_uuid.h"

static int tests_run = 0;
static int tests_ok  = 0;

#define CHECK(cond, msg) do { \
    tests_run++; \
    if (cond) { tests_ok++; printf("OK  %s\n", msg); } \
    else { printf("FAIL %s (line %d)\n", msg, __LINE__); } \
} while(0)

static int valid_uuid(const char *s)
{
    /* Format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx */
    if (strlen(s) != 36) return 0;
    const int dashes[] = {8, 13, 18, 23, -1};
    for (int i = 0; dashes[i] >= 0; i++)
        if (s[dashes[i]] != '-') return 0;
    for (int i = 0; i < 36; i++) {
        if (i == 8 || i == 13 || i == 18 || i == 23) continue;
        char c = s[i];
        if (!((c >= '0' && c <= '9') ||
              (c >= 'a' && c <= 'f') ||
              (c >= 'A' && c <= 'F'))) return 0;
    }
    return 1;
}

int main(void)
{
    char uuid1[37], uuid2[37];

    int r1 = trackterm_uuid_generate(uuid1);
    int r2 = trackterm_uuid_generate(uuid2);

    CHECK(r1 == 0, "generate uuid1");
    CHECK(r2 == 0, "generate uuid2");
    CHECK(valid_uuid(uuid1), "uuid1 format valid");
    CHECK(valid_uuid(uuid2), "uuid2 format valid");
    CHECK(strcmp(uuid1, uuid2) != 0, "uuid1 != uuid2 (uniqueness)");

    printf("UUID1: %s\n", uuid1);
    printf("UUID2: %s\n", uuid2);

    printf("\n%d/%d tests passed\n", tests_ok, tests_run);
    return (tests_ok == tests_run) ? 0 : 1;
}
