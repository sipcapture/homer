AC_PREREQ(2.57)
AC_INIT(homer, 3.2.5, homer-discuss@googlegroups.com)
AC_CONFIG_AUX_DIR(config)
AM_INIT_AUTOMAKE(foreign tar-ustar)
AC_PROG_CC

if test -f /etc/debian_version ; then
    WEBROOT='$(localstatedir)/www'
elif test -f /etc/redhat-release ; then
    WEBROOT='$(localstatedir)/www/html'
elif test -f /etc/SuSE-release ; then
    WEBROOT='$(prefix)/srv/www/htdocs'
fi

AC_ARG_WITH([lighttpd],
  [AS_HELP_STRING([--with-lighttpd Configure and install with lighttpd settings. Otherwise apache is assumed])],
[
  dnl Not sure about these settings --Douglas
  AC_SUBST(WEBUSER, lighttpd)
  AC_SUBST(WEBUSER, lighttpd)
  AC_SUBST(WEBROOT)
],[
  AC_SUBST(WEBUSER, apache)
  AC_SUBST(WEBGROUP, apache)
  AC_SUBST(WEBROOT)
])

AC_CONFIG_FILES([
    Makefile
    captagent/Makefile
    webhomer/Makefile
    scripts/Makefile
])
AC_OUTPUT
