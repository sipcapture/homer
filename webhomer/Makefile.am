EXTRA_DIST = $(nobase_web_DATA)

webdir = $(WEBDIR)
nobase_web_DATA = \
	$(shell cd $(srcdir); find . \( \
	  -not -name 'Makefile*' \
	  -a -not -name 'COPYING' \
	  -a -not -name 'README' \
	\) -type f -printf '%P\n')

#
#install-data-hook :
#	chown $(WEBUSER):$(WEBGROUP) $(DESTDIR)$(webdir)/tmp
