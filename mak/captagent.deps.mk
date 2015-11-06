# Copyright (C) Homer Project 2012-2015 (http://www.sipcapture.org).
# Licensed to the User under the GPL license.
#

captagent_VER = 6.0.1
captagent_TAG = 6.0.1
captagent_BRANCH = captagent6
captagent_PACKAGE_REVISION = $(shell cd $(SRC)/captagent; git checkout $(captagent_BRANCH) >/dev/null 2>&1; ../config/revision-gen $(captagent_TAG))
captagent_SRPM = captagent-$(captagent_VER)-$(captagent_PACKAGE_REVISION).src.rpm
captagent_TAR = captagent/captagent-$(captagent_VER).tar.gz
captagent_DEB = deb
captagent_APT_SETUP = libmysqlclient-dev libexpat1-dev bison flex libpcap-dev libjson-c-dev

captagent_SRPM_DEFS = \
	--define "BUILD_NUMBER $(captagent_PACKAGE_REVISION)" \
	--define "VERSION_NUMBER $(captagent_VER)"

captagent_RPM_DEFS = \
	--define="BUILD_NUMBER $(captagent_PACKAGE_REVISION)" \
	--define "VERSION_NUMBER $(captagent_VER)"
