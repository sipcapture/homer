EXTRA_DIST = $(nobase_web_DATA)

webdir = $(WEBROOT)/webhomer
nobase_web_DATA = \
	$(shell cd $(srcdir); find . \( \
	  -not -name 'Makefile*' \
	  -a -not -name 'COPYING' \
	  -a -not -name 'README' \
	\) -type f -printf '%P\n')
