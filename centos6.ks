install
text
sshpw --username=root setup --plaintext
cdrom

keyboard us
lang en_US.UTF8

timezone --utc Europe/Amsterdam

auth --enableshadow --passalgo=sha512 --enablefingerprint
rootpw --iscrypted $6$y4oi7guFydjyif96$9bm7hVh/D5DxqAF1vEAPCZKSUdF3GiLFP2lcui9uGZ6tCKYr30c20gIBtU8jO08IYmG9Frrz1d.XmMktmYoPp.

selinux --disabled
firewall --disabled

ignoredisk --only-use=sda
clearpart --drives=sda --all --initlabel
zerombr

part /boot --size=1024 --ondrive=sda --asprimary --fstype=ext4
part pv.01 --size=1    --ondrive=sda --grow

volgroup vg_main pv.01
logvol swap  --vgname=vg_main --size=1024   --name=swap --fstype=swap
logvol /     --vgname=vg_main --size=2048   --name=root --fstype=ext4 --fsoptions="defaults,sync,relatime"
logvol /home --vgname=vg_main --size=1024   --name=home --fstype=ext4 --fsoptions="defaults,nodev"
logvol /usr  --vgname=vg_main --size=4096   --name=usr  --fstype=ext4 --fsoptions="defaults,nodev"
logvol /var  --vgname=vg_main --size=1      --name=var  --fstype=ext4 --fsoptions="defaults,nodev,nosuid,relatime" --grow
bootloader --location=mbr

%packages --nobase
@Core

%end

%post —log=/var/log/ks-post.log

systemctl enable sshd.service

%end

reboot
