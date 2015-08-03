PROJ_NAME = kamailio
$(PROJ_NAME)_VER = 4.3.1
$(PROJ_NAME)_TAG = 4.3.1
$(PROJ_NAME)_PACKAGE_REVISION = $(shell cd $(SRC)/$(PROJ_NAME); ../config/revision-gen $($(PROJ_NAME)_TAG))
$(PROJ_NAME)_SRPM = $(PROJ_NAME)-$($(PROJ_NAME)_VER)-$($(PROJ_NAME)_PACKAGE_REVISION).src.rpm
$(PROJ_NAME)_TAR = $(PROJ_NAME)/$(PROJ_NAME)-$($(PROJ_NAME)_VER)_src.tar.gz
$(PROJ_NAME)_SPEC = distr/el/$(PROJ_NAME).spec

$(PROJ_NAME)_SRPM_DEFS = \
	--define "BUILD_NUMBER $($(PROJ_NAME)_PACKAGE_REVISION)" \
	--define "VERSION_NUMBER $($(PROJ_NAME)_VER)"

$(PROJ_NAME)_RPM_DEFS = \
	--define="BUILD_NUMBER $($(PROJ_NAME)_PACKAGE_REVISION)" \
	--define "VERSION_NUMBER $($(PROJ_NAME)_VER)"

kamailio.autoreconf:
	@echo "========= autoreconf ===============" 1>&2

kamailio.configure:
	@echo "========= configure ===============" 1>&2
	test -d $(PROJ_NAME) || mkdir -p $(PROJ_NAME)
	cp -r $(SRC)/$(PROJ_NAME)/* $(PROJ_NAME)
	rm -rf $(PROJ_NAME)/pkg
	cp $($(PROJ_NAME)_SPEC) $(PROJ_NAME)
