package main

// Provide stubs for glibc symbols that were introduced after GLIBC 2.28
// (the version shipped with RHEL 8 / AlmaLinux 8 / Rocky Linux 8).
// These stubs allow polyfill-glibc to lower the minimum glibc requirement
// to 2.28 so the binary runs on RHEL 8 without error.
//
// _dl_find_object  — added in glibc 2.35; used by libgcc's _Unwind_Find_FDE
//                    for exception unwinding. The weak stub below is compiled
//                    only when building against glibc < 2.35 headers (e.g.
//                    RHEL 8 / glibc 2.28); on those builds the real symbol may
//                    be found via dlsym at runtime and proxied, or -1 is
//                    returned to trigger libgcc's legacy FDE lookup fallback.
//                    On glibc 2.35+ the stub is suppressed entirely to avoid a
//                    "conflicting types" error — the real symbol is used directly.
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

// Ensure __GLIBC_PREREQ is defined on non-glibc toolchains (e.g. musl).
#if !defined(__GLIBC_PREREQ)
# define __GLIBC_PREREQ(maj, min) 0
#endif

// _dl_find_object was introduced in glibc 2.35 and is declared in <dlfcn.h>
// on those systems using the public type |struct dl_find_object|.  On older
// glibc (e.g. RHEL 8 / glibc 2.28) neither the header declaration nor the
// library symbol exist, so we must provide a weak stub.
//
// Guard with __GLIBC_PREREQ to avoid redeclaring the function when the
// system headers already provide a declaration — redeclaring with a different
// struct name (struct _dl_find_object_result vs. struct dl_find_object)
// triggers a "conflicting types" compiler error on Ubuntu 24.04 ARM64
// (glibc 2.38) and other platforms shipping glibc 2.35+.
#if !__GLIBC_PREREQ(2, 35)
// Forward-declare the public struct introduced in glibc 2.35.  The stub
// never accesses any fields, so an incomplete type is sufficient here.
struct dl_find_object;

typedef int (*_dl_find_object_fn)(void *, struct dl_find_object *);

// Weak stub: the strong libc symbol overrides this at link time on glibc
// 2.35+ systems, so it is never called there.  On older glibc (no real
// symbol at link time), the dlsym probe below tries to find the real
// implementation at runtime (e.g. a polyfill-patched binary running on a
// newer system); failing that, returning -1 lets libgcc fall back to the
// legacy FDE lookup path for exception unwinding.
__attribute__((weak))
int _dl_find_object(void *address, struct dl_find_object *result)
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
#endif /* !__GLIBC_PREREQ(2, 35) */

// __libc_single_threaded stub — must be a writable data symbol (char).
// Initialise to 0 (multi-threaded) so locks are never skipped.
__attribute__((weak))
char __libc_single_threaded = 0;
*/
import "C"
