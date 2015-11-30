kamailio_VER = $(shell cd $(SRC)/kamailio; grep -A4 ' version number' Makefile.defs | grep -E '^VERSION' | sed 's/ //g' | sed 's/VERSION=//g' ).$(shell cd $(SRC)/kamailio; grep -A4 ' version number' Makefile.defs | grep -E '^PATCHLEVEL' | sed 's/ //g' | sed 's/PATCHLEVEL=//g' ).$(shell cd $(SRC)/kamailio; grep -A4 ' version number' Makefile.defs | grep -E '^SUBLEVEL' | sed 's/ //g' | sed 's/SUBLEVEL=//g' )$(shell cd $(SRC)/kamailio; grep -A4 ' version number' Makefile.defs | grep -E '^EXTRAVERSION' | sed 's/ //g' | sed 's/EXTRAVERSION=//g' )
kamailio_TAG = $(kamailio_VER)
kamailio_BRANCH = 4.3
kamailio_PACKAGE_REVISION = $(shell cd $(SRC)/kamailio; git checkout $(kamailio_BRANCH) >/dev/null 2>&1; ../config/revision-gen $(kamailio_TAG))$(RPM_DIST)
kamailio_SRPM = kamailio-$(kamailio_VER)-$(kamailio_PACKAGE_REVISION).src.rpm
kamailio_TAR = kamailio/kamailio-$(kamailio_VER)_src.tar.gz
kamailio_SPEC = distr/el/kamailio.spec
kamailio_DEPS = libxmlrpc-core-c3-dev
kamailio_APT_SETUP = docbook-xml erlang-dev gcj-jdk libconfuse-dev libcurl4-openssl-dev libdb-dev libev-dev libevent-dev libfreeradius-client-dev libgeoip-dev libhiredis-dev libjansson-dev libldap2-dev liblua5.1-0-dev libmemcached-dev libmono-2.0-dev libncurses5-dev libpcre3-dev libperl-dev libpq-dev libpurple-dev librabbitmq-dev libreadline-dev libsasl2-dev libsctp-dev libsnmp-dev libsqlite3-dev libunistring-dev libval-dev libxml2-dev pkg-config python-dev unixodbc-dev uuid-dev xsltproc

kamailio_SRPM_DEFS = \
	--define "BUILD_NUMBER $(kamailio_PACKAGE_REVISION)" \
	--define "VERSION_NUMBER $(kamailio_VER)" \
	--define "SRC_TAR kamailio-$(kamailio_VER)_src.tar.gz"

kamailio_RPM_DEFS = \
	--define="BUILD_NUMBER $(kamailio_PACKAGE_REVISION)" \
	--define "VERSION_NUMBER $(kamailio_VER)" \
	--define "SRC_TAR kamailio-$(kamailio_VER)_src.tar.gz"

kamailio.autoreconf:
	@echo -n

kamailio.configure:
	test -d kamailio || mkdir -p kamailio
	cp -r $(SRC)/kamailio/* kamailio
	rm -rf kamailio/pkg
	cp $(kamailio_SPEC) kamailio
	cp distr/el/kamailio.logrotate kamailio
	cp distr/el/kamailio.rsyslog kamailio
	cp distr/el/el7/kamailio.sysconfig kamailio/kamailio.sysconfig.el7
	cp distr/el/el7/kamailio.service kamailio/kamailio.service.el7
	cp distr/el/el6/kamailio.sysconfig kamailio/kamailio.sysconfig.el6
	cp distr/el/el6/kamailio.init kamailio/kamailio.init.el6

kamailio.distdir: kamailio.configure
	@echo "make distdir for kamailio"
	mkdir -p kamailio/pkg/debian
	cp -r $(SRC)/kamailio/pkg/kamailio/deb/debian/* kamailio/pkg/debian/
	sed -i s/libxmlrpc-c3-dev/libxmlrpc-core-c3-dev/g kamailio/pkg/debian/control

kamailio.deb-setup:
	@echo "Installing development packages"
	sudo apt-get install $(kamailio_DEPS) -y
