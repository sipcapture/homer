# Initial Version Copyright (C) 2011 eZuce, Inc., All Rights Reserved.
# Licensed to the User under the LGPL license.
#
#
# Copyright (C) Homer Project 2012-2015 (http://www.sipcapture.org).
# Licensed to the User under the GPL license.
#
#
# common directories and variables used in project
#
AC_PREFIX_DEFAULT([/usr/local])

# This is a "common fix" to a "known issue" with using ${prefix} variable
test "x$prefix" = xNONE && prefix=$ac_default_prefix
test "x$exec_prefix" = xNONE && exec_prefix='${prefix}'

AC_SUBST(PROJECT_INCDIR, [${includedir}])
AC_SUBST(PROJECT_LIBDIR, [${libdir}])
AC_SUBST(PROJECT_LIBEXECDIR, [${libexecdir}/sipcapture])
AC_SUBST(PROJECT_BINDIR,  [${bindir}])
AC_SUBST(PROJECT_CONFDIR, [${sysconfdir}/sipcapture])
AC_SUBST(PROJECT_DATADIR, [${datadir}/sipcapture])
AC_SUBST(PROJECT_DOCDIR,  [${datadir}/doc/sipcapture])
AC_SUBST(PROJECT_VARDIR,  [${localstatedir}/sipcapture])
AC_SUBST(PROJECT_TMPDIR,  [${localstatedir}/sipcapture/tmp])
AC_SUBST(PROJECT_LOGDIR,  [${localstatedir}/log/sipcapture])
AC_SUBST(PROJECT_RUNDIR,  [${localstatedir}/run/sipcapture])
AC_SUBST(PROJECT_VARLIB,  [${localstatedir}/lib/sipcapture])
AC_SUBST(PROJECT_SERVICEDIR, [${sysconfdir}/init.d])
# Project RPMs should be hardcoded to use sipcapture user for their runtime user, not the buildbot user
AC_SUBST(PROJECT_RPM_CONFIGURE_OPTIONS,  [PROJECTUSER=sipcapture])

# Get the user to run Project applications under.
AC_ARG_VAR(PROJECTUSER, [The project applications service daemon user name, default is ${USER}])
test -n "$PROJECTUSER" || PROJECTUSER=$USER

# Get the group to run Project applications under.
AC_ARG_VAR(PROJECTGROUP, [The project applications service daemon group name, default is value of PROJECTUSER])
test -n "$PROJECTGROUP" || PROJECTGROUP=$PROJECTUSER

AC_ARG_VAR(PACKAGE_REVISION, [Package revision number. Default is based on date rpm is built. Allowed values: stable, unstable, developer or supply your own value of command.])
if test -z "$PACKAGE_REVISION" || test "$PACKAGE_REVISION" == "developer" ; then
  PACKAGE_REVISION=`date +%Y%m%d%H%M%S`
else
  case ${PACKAGE_REVISION} in
    unstable )
      PACKAGE_REVISION=0`date +%Y%m%d%H%M%S`
      ;;
    stable )
      PACKAGE_REVISION=`cd ${srcdir} && ./config/revision-gen ${PACKAGE_VERSION}`
      ;;
    esac
fi
AC_DEFINE_UNQUOTED([PACKAGE_REVISION], "${PACKAGE_REVISION}", [Revion number including git SHA])

# automake eats straight "if.." in makefiles as autoconf conditions. this avoids that
# http://bugs.debian.org/cgi-bin/bugreport.cgi?bug=34051
AC_SUBST(IF, [if])
AC_SUBST(IFDEF, [ifdef])
AC_SUBST(IFNDEF, [ifndef])
AC_SUBST(ENDIF, [endif])

