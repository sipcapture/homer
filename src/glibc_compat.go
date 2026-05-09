package main

// Provide stubs for glibc symbols that were introduced after GLIBC 2.28
// (the version shipped with RHEL 8 / AlmaLinux 8 / Rocky Linux 8).
// These stubs allow polyfill-glibc to lower the minimum glibc requirement
// to 2.28 so the binary runs on RHEL 8 without error.
//
// _dl_find_object  — added in glibc 2.35; used by libgcc's _Unwind_Find_FDE
//                    for exception unwinding. On modern systems (Debian 13,
//                    Ubuntu 22.04+) the real glibc implementation is called via
//                    dlsym so that DuckDB's C++ exception handling works correctly.
//                    On RHEL 8 (glibc 2.28, no real implementation) returns -1,
//                    triggering graceful fallback to the legacy FDE lookup path.
//
// __libc_single_threaded — added in glibc 2.32; a global flag used by mutex
//                    fast-paths. Defaulting to 0 (multi-threaded) is safe.

/*
#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif
#include <dlfcn.h>
#include <errno.h>
#include <stddef.h>

// We define the result struct ourselves to avoid requiring glibc 2.35 headers.
struct _dl_find_object_result { void *dlpi_name; };

typedef int (*_dl_find_object_fn)(void *, struct _dl_find_object_result *);

// _dl_find_object stub — on systems with glibc 2.35+ the real implementation
// is resolved at runtime via dlsym and called, preserving correct C++ exception
// unwinding (critical for DuckDB's JIT-compiled query code).
// On RHEL 8 (glibc 2.28) the real symbol is absent; returning -1 triggers
// libgcc's legacy FDE lookup fallback.
__attribute__((weak))
int _dl_find_object(void *address, struct _dl_find_object_result *result)
{
    static _dl_find_object_fn real_fn = (_dl_find_object_fn)0;
    static int resolved = 0;

    if (!resolved) {
        resolved = 1;
        void *sym = dlsym(RTLD_NEXT, "_dl_find_object");
        // Guard against self-reference (some linker configurations may return
        // a pointer back to this weak definition).
        if (sym && sym != (void *)_dl_find_object) {
            real_fn = (_dl_find_object_fn)sym;
        }
    }
    if (real_fn) {
        return real_fn(address, result);
    }
    errno = ENOSYS;
    return -1;
}

// __libc_single_threaded stub — must be a writable data symbol (char).
// Initialise to 0 (multi-threaded) so locks are never skipped.
__attribute__((weak))
char __libc_single_threaded = 0;
*/
import "C"
