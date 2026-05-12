//go:build linux && cgo

// Package glibcstub provides a weak _dl_find_object implementation linked from a
// small separate archive so it can satisfy libgcc's undefined reference before
// libc's strong GLIBC_2.35+ symbol is chosen — keeping the executable polyfillable
// down to glibc 2.34. (Defining this in the same cgo unit as other C code was
// not sufficient on Ubuntu 24.04+.)
package glibcstub

/*
#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif
#include <errno.h>
#include <stddef.h>

#if !defined(__GLIBC_PREREQ)
#define __GLIBC_PREREQ(maj, min) 0
#endif

#if defined(__GLIBC__) && __GLIBC_PREREQ(2, 35)
#include <dlfcn.h>

typedef int (*homer_dl_find_object_fn)(void *, struct dl_find_object *);

__attribute__((weak))
int _dl_find_object(void *__address, struct dl_find_object *__result)
{
	static homer_dl_find_object_fn real_fn;
	static int resolved;

	if (!resolved) {
		resolved = 1;
		void *sym = dlsym(RTLD_NEXT, "_dl_find_object");
		if (sym && sym != (void *)_dl_find_object) {
			real_fn = (homer_dl_find_object_fn)sym;
		}
	}
	if (real_fn) {
		return real_fn(__address, __result);
	}
	errno = ENOSYS;
	return -1;
}

#elif defined(__GLIBC__)

struct dl_find_object;

typedef int (*homer_dl_find_object_fn)(void *, struct dl_find_object *);

__attribute__((weak))
int _dl_find_object(void *address, struct dl_find_object *result)
{
	static homer_dl_find_object_fn real_fn;
	static int resolved;

	if (!resolved) {
		resolved = 1;
		void *sym = dlsym(RTLD_NEXT, "_dl_find_object");
		if (sym && sym != (void *)_dl_find_object) {
			real_fn = (homer_dl_find_object_fn)sym;
		}
	}
	if (real_fn) {
		return real_fn(address, result);
	}
	errno = ENOSYS;
	return -1;
}

#endif
*/
import "C"
