kamailio_VER = 4.3.1
kamailio_TAG = 4.3.1
kamailio_PACKAGE_REVISION = $(shell cd $(SRC)/kamailio; ../config/revision-gen $(kamailio_TAG))
kamailio_SRPM = kamailio-$(kamailio_VER)-$(kamailio_PACKAGE_REVISION).src.rpm
kamailio_TAR = kamailio/kamailio-$(kamailio_VER)_src.tar.gz
kamailio_SPEC = distr/el/kamailio.spec

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
	cp distr/el/kamailio.sysconfig kamailio
	cp distr/el/kamailio.service kamailio
