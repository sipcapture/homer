package main

// Provide stubs for glibc symbols that were introduced after GLIBC 2.28
// (the version shipped with RHEL 8 / AlmaLinux 8 / Rocky Linux 8).
// These stubs allow polyfill-glibc to lower the minimum glibc requirement
// to 2.28 so the binary runs on RHEL 8 without error.
//
// _dl_find_object — see package glibcstub (linked early on linux+cgo) so
// polyfill-glibc can lower the binary to glibc 2.34 without an unresolved
// _dl_find_object@GLIBC_2.35 reference from the main cgo translation unit.
//
// __libc_single_threaded — added in glibc 2.32; a global flag used by mutex
//                    fast-paths. Defaulting to 0 (multi-threaded) is safe.

/*
#ifndef _GNU_SOURCE
#define _GNU_SOURCE
#endif
#include <errno.h>
#include <stddef.h>

// __libc_single_threaded stub — must be a writable data symbol (char).
// Initialise to 0 (multi-threaded) so locks are never skipped.
__attribute__((weak))
char __libc_single_threaded = 0;
*/
import "C"
