#
# makefile
#

bin_PROGRAMS = captagent

captagent_LDFLAGS = \
	-lpcap

captagent_CFLAGS = \
	-g -Wall -DUSE_IPV6 -DUSE_HEP2 -DUSE_CONFFILE

captagent_SOURCES = \
	minIni/minIni.c \
	captagent.c

noinst_HEADERS = \
	captagent.h \
	minIni/minGlue-ccs.h \
	minIni/minGlue-efsl.h \
	minIni/minGlue-FatFs.h \
	minIni/minGlue-ffs.h \
	minIni/minGlue.h \
	minIni/minGlue-mdd.h \
	minIni/minGlue-stdio.h \
	minIni/minIni.h \
	minIni/wxMinIni.h

confdir = $(sysconfdir)/captagent
dist_conf_DATA = \
	captagent.ini
