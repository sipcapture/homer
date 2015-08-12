dnl Initial Version Copyright (C) Homer Project 2012-2015 (http://www.sipcapture.org).
dnl Licensed to the User under the GPL license.
dnl

AC_ARG_ENABLE(iso, [--enable-iso Build @PACKAGE_NAME@ or custom ISO image],
[
    if test "$SETUP_TARGET" != "rpm" ; then
	AC_MSG_ERROR([RPMs to be belt required. Use --enable-rpm for configure])
    fi

    AC_PATH_PROG(MKISOFS,mkisofs)
    if [ test -z "$MKISOFS" ]; then
	AC_MSG_ERROR([mkisofs program is required. Redhat users run: 'yum install genisoimage'])
    fi

    AC_CONFIG_FILES([mak/35-iso.mk:mak/iso.mk.in])
])
