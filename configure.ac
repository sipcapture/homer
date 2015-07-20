# Initial Version Copyright (C) 2010 eZuce, Inc., All Rights Reserved.
# Licensed to the User under the LGPL license.
#
#
# Copyright (C) Homer Project 2012-2015 (http://www.sipcapture.org).
# Licensed to the User under the GPL license.
#
#
AC_PREREQ(2.61)

AC_INIT(homer,5.0.1,support@sipcapture.org,,http://www.sipcapture.org)

# Pass standard and sipxecs common params to ./configure switches on all sub-projects.
# Doesn't have to handle ./configure vars because those can be passed handled
OPTIONS=
for a in $@; do
  case $a in
    --prefix*)OPTIONS+=" $a" ;;
    --disable-dep-check*)OPTIONS+=" $a" ;;
    --enable-debug*)OPTIONS+=" $a" ;;
    PACKAGE_REVISION*)OPTIONS+=" $a" ;;
  esac
done
AC_SUBST(OPTIONS)

m4_include([config/projectlib2.m4])
m4_include([mak/rpm.m4])
#m4_include([mak/centos-iso.m4])
m4_sinclude([custom/custom.m4])

AC_CONFIG_FILES([Makefile])
AC_CONFIG_FILES([mak/05-build.mk:mak/build.mk.in])
AC_CONFIG_FILES([mak/40-modules.mk:mak/modules.mk.in])
AC_CONFIG_FILES([mak/20-list-dependencies.mk:mak/list-dependencies.mk.in])
AC_CONFIG_FILES([mak/30-setup.mk:mak/setup.mk.in])

AC_OUTPUT
