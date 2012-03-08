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

nobase_include_HEADERS = \
	captagent.h

confdir = $(sysconfdir)/captagent
dist_conf_DATA = \
	captagent.ini
