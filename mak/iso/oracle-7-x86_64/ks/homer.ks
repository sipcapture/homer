install
cdrom

keyboard us
lang en_US.UTF8

timezone --utc Asia/Novosibirsk

auth --enableshadow --passalgo=sha512 --enablefingerprint
rootpw --iscrypted $6$y4oi7guFydjyif96$9bm7hVh/D5DxqAF1vEAPCZKSUdF3GiLFP2lcui9uGZ6tCKYr30c20gIBtU8jO08IYmG9Frrz1d.XmMktmYoPp.

selinux --disabled
firewall --disabled

ignoredisk --only-use=sda
clearpart --drives=sda --all --initlabel
zerombr
part /boot --fstype=ext4 --size=1024 --asprimary --ondrive=sda
part pv.01 --size=1 --grow --ondrive=sda
volgroup vg_main pv.01
logvol /     --vgname=vg_main --size=2048   --name=root --label=ROOT --fstype=ext4 --fsoptions="defaults,sync,relatime"
logvol /home --vgname=vg_main --size=1024   --name=home --label=HOME --fstype=ext4 --fsoptions="defaults,nodev"
logvol /opt  --vgname=vg_main --size=2048   --name=opt  --label=OPT  --fstype=ext4 --fsoptions="defaults,nodev"
logvol /tmp  --vgname=vg_main --size=1024   --name=tmp  --label=TMP  --fstype=ext4 --fsoptions="nosuid,noatime,data=writeback,barrier=0,nobh,errors=remount-ro"
logvol /usr  --vgname=vg_main --size=4096   --name=usr  --label=USR  --fstype=ext4 --fsoptions="defaults,nodev"
logvol /var  --vgname=vg_main --size=1      --name=var  --label=VAR  --fstype=ext4 --fsoptions="defaults,nodev,nosuid,relatime" --grow
logvol swap  --vgname=vg_main --recommended --name=swap --label=SWAP --fstype=swap
bootloader --location=mbr

%packages
@core
%end

%post —log=/var/log/ks-post.log

systemctl enable sshd.service

%end

reboot
