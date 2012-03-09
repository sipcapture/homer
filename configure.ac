AC_PREREQ(2.57)
AC_INIT(homer, 0.9, homer-discuss@googlegroups.com)
AC_CONFIG_AUX_DIR(config)
AM_INIT_AUTOMAKE(foreign tar-ustar)
AC_PROG_CC

AC_ARG_WITH([lighttpd],
  [AS_HELP_STRING([--with-lighttpd Configure and install with lighttpd settings. Otherwise apache is assumed])],
[
  dnl Not sure about these settings --Douglas
  AC_SUBST(WEBUSER, lighttpd)
  AC_SUBST(WEBUSER, lighttpd)
  AC_SUBST(WEBDIR, ['$(localstatedir)/www/webhomer'])
],[
  AC_SUBST(WEBUSER, apache)
  AC_SUBST(WEBGROUP, apache)
  AC_SUBST(WEBDIR, ['$(localstatedir)/www/webhomer'])
])

AC_CONFIG_FILES([
    Makefile
    captagent/Makefile
    webhomer/Makefile
])
AC_OUTPUT
