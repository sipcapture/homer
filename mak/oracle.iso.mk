# Initial Version Copyright (C) Homer Project 2012-2015 (http://www.sipcapture.org).
# Licensed to the User under the GPL license.
#

iso_oracle_minimal_group = base

oracle-7-x86_64.iso.packages :
	yumdownloader --destdir iso/$(DISTRO)/Packages \@$(iso_oracle_minimal_group)
