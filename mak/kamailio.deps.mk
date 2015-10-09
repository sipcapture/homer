kamailio_VER = 4.3.1
kamailio_TAG = 4.3.1
kamailio_PACKAGE_REVISION = $(shell cd $(SRC)/kamailio; ../config/revision-gen $(kamailio_TAG))
kamailio_SRPM = kamailio-$(kamailio_VER)-$(kamailio_PACKAGE_REVISION).src.rpm
kamailio_TAR = kamailio/kamailio-$(kamailio_VER)_src.tar.gz
kamailio_SPEC = distr/el/kamailio.spec
kamailio_DEPS = libxmlrpc-core-c3-dev

kamailio_SRPM_DEFS = \
	--define "BUILD_NUMBER $(kamailio_PACKAGE_REVISION)" \
	--define "VERSION_NUMBER $(kamailio_VER)"

kamailio_RPM_DEFS = \
	--define="BUILD_NUMBER $(kamailio_PACKAGE_REVISION)" \
	--define "VERSION_NUMBER $(kamailio_VER)"

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
