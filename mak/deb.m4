dnl Copyright (C) Homer Project 2012-2015 (http://www.sipcapture.org).
dnl Licensed to the User under the GPL license.
dnl
dnl Author: Konstantin S. Vishnivetsky
dnl

AC_CHECK_FILE(/usr/bin/dpkg-buildpackage,
[
  DEB_DIST=`cat /etc/debian_version`
  AC_SUBST(DEB_DIST)

  DistroArchDefault=`uname -m`

  dnl NOTE LIMITATION: default doesn't account for distros besides centos, redhat and suse or redhat 6
  DistroOsDefault=`debian`

  dnl NOTE LIMITATION: default doesn't account for distros besides centos, redhat and suse or redhat 6
  DistroVerDefault=`jessie`

  DistroDefault="${DistroOsDefault}-${DistroVerDefault}-${DistroArchDefault}"

  AllDistrosDefault="debian-jessie-i386 debian-jessie-x86_64 debian-sid-i386 debian-sid-x86_64"
])
AC_ARG_ENABLE(deb, [--enable-deb Use build toold to build DEBs],
[

  AC_ARG_VAR(DISTRO, [What operating system you are compiling for. Default is ${DistroDefault}])
  test -n "${DISTRO}" || DISTRO="debian-jessie-x86_64"

  AC_ARG_VAR(ALL_DISTROS, [All distros which using cross distroy compiling (xc.* targets) Default is ${AllDistrosDefault}])
  test -n "${ALL_DISTROS}" || ALL_DISTROS="${AllDistrosDefault}"

  SETUP_TARGET=deb
  AC_CONFIG_FILES([mak/10-deb.mk:mak/deb.mk.in])
])
