# Initial Version Copyright (C) Homer Project 2012-2015 (http://www.sipcapture.org).
# Licensed to the User under the GPL license.
#

iso_oracle_minimal_packages = \
authconfig \
grub2 \
firewalld \
lvm2 \
kernel-uek \
kernel

iso_oracle_minimal_groups = \
core

oracle-7-x86_64.iso.packages :
	yumdownloader -y --installroot=/tmp --resolve --destdir iso/$(DISTRO)/Packages \@$(iso_oracle_minimal_groups) $(iso_oracle_minimal_packages)
