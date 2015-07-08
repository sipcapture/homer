# Definitions
%define debug_package %{nil}
%if %{_vendor} == suse
%define webroot %{_prefix}/srv/%name/htdocs
%else # CentOS/Fedora
%define webroot %{_localstatedir}/www/%name
%define webuser apache
%define webgroup apache
%endif
%if 0%{?rhel} < 6 && 0%{?fedora} == 0
%define php php53
# part of json support is now in php-common
Requires: %{php}-json
%else
%define php php
%endif

Name:		homer
Version:	5.0.1
Release:	0
Summary:	SIP capture server

Group:		Applications/Communications
License:	GPLv3
URL:		http://www.sipcapture.org/
Source0:	%name-%version.tar.gz

%description
HOMER is a robust, carrier-grade, scalable SIP Capture system and Monitoring Application with HEP/HEP2, IP Proto4 (IPIP) encapsulation & port mirroring/monitoring support right out of the box, ready to process & store insane amounts of signaling with instant search, end-to-end analysis and drill-down capabilities for ITSPs, VoIP Providers and Trunk Suppliers using SIP signaling

%package api
Summary:	HTTP based API for HOMER
Requires:	%name >= %version
Requires:	ntp
Requires:	%{php} >= 5
Requires:	php-mysql
Requires:	mysql-community-client >= 5.6

%description api
HTTP based API for HOMER is a robust, carrier-grade, scalable SIP Capture system and Monitoring Application with HEP/HEP2, IP Proto4 (IPIP) encapsulation & port mirroring/monitoring support right out of the box, ready to process & store insane amounts of signaling with instant search, end-to-end analysis and drill-down capabilities for ITSPs, VoIP Providers and Trunk Suppliers using SIP signaling

%package ui
Summary:	WEB UI for HOMER
Requires:	%name-api >= %version

%description ui
WEB UI for HOMER is a robust, carrier-grade, scalable SIP Capture system and Monitoring Application with HEP/HEP2, IP Proto4 (IPIP) encapsulation & port mirroring/monitoring support right out of the box, ready to process & store insane amounts of signaling with instant search, end-to-end analysis and drill-down capabilities for ITSPs, VoIP Providers and Trunk Suppliers using SIP signaling

%package nginx
Summary:	nginx vhost config for HOMER
Requires:	%name >= %version
Requires:	nginx
Conflicts:	%name-apache

%description nginx
Nginx vhost config for HOMER is a robust, carrier-grade, scalable SIP Capture system and Monitoring Application with HEP/HEP2, IP Proto4 (IPIP) encapsulation & port mirroring/monitoring support right out of the box, ready to process & store insane amounts of signaling with instant search, end-to-end analysis and drill-down capabilities for ITSPs, VoIP Providers and Trunk Suppliers using SIP signaling

%package apache
Summary:	Apache(httpd) vhost config for HOMER
Requires:	%name >= %version
Requires:	httpd
Conflicts:	%name-nginx

%description apache
Apache(httpd) vhost config for HOMER is a robust, carrier-grade, scalable SIP Capture system and Monitoring Application with HEP/HEP2, IP Proto4 (IPIP) encapsulation & port mirroring/monitoring support right out of the box, ready to process & store insane amounts of signaling with instant search, end-to-end analysis and drill-down capabilities for ITSPs, VoIP Providers and Trunk Suppliers using SIP signaling

%package mysql
Summary:	MySQL schemas for HOMER
Requires:	%name >= %version
Requires:	mysql-community-server >= 5.6
Requires:	perl-DBD-MySQL

%description mysql
MySQL schemas for HOMER is a robust, carrier-grade, scalable SIP Capture system and Monitoring Application with HEP/HEP2, IP Proto4 (IPIP) encapsulation & port mirroring/monitoring support right out of the box, ready to process & store insane amounts of signaling with instant search, end-to-end analysis and drill-down capabilities for ITSPs, VoIP Providers and Trunk Suppliers using SIP signaling

%package kamailio
Summary:	kamailio config for HOMER
Requires:	%name >= %version
Requires:	ntp
Requires:	kamailio
Requires:	kamailio-mysql

%description kamailio
Kamailio config for HOMER is a robust, carrier-grade, scalable SIP Capture system and Monitoring Application with HEP/HEP2, IP Proto4 (IPIP) encapsulation & port mirroring/monitoring support right out of the box, ready to process & store insane amounts of signaling with instant search, end-to-end analysis and drill-down capabilities for ITSPs, VoIP Providers and Trunk Suppliers using SIP signaling

%files
%defattr(0660,root,root)
%dir %{_docdir}/%name
%dir %{_datadir}/%name
%doc %{_docdir}/%name/COPYING
%doc %{_docdir}/%name/README

%files kamailio
%defattr(0644,root,root)
%dir %attr(2770,kamailio,kamailio)%{_localstatedir}/run/sipcapture
%{_sysconfdir}/kamailio/kamailio-sipcapture.cfg
%{_sysconfdir}/sysconfig/sipcapture
%{_unitdir}/sipcapture.service
%{_sysconfdir}/systemd/system/multi-user.target.wants/sipcapture.service
%{_sysconfdir}/rsyslog.d/sipcapture.conf
%{_sysconfdir}/logrotate.d/sipcapture

%files apache
%defattr(0644,root,root)
%config(noreplace)%{_sysconfdir}/httpd/conf.d/%name.conf

%files mysql
%defattr(0644,root,root)
%dir %{_datadir}/%name/sql
%{_datadir}/%name/sql/*
%attr(0770,root,root)%{_bindir}/homer_mysql_new_table.pl
%attr(0770,root,root)%{_bindir}/homer_mysql_partrotate_unixtimestamp.pl
%attr(0770,root,root)%{_bindir}/homer_mysql_rotate
%{_sysconfdir}/cron.daily/homer_mysql_logrotate

%files nginx
%defattr(0644,root,root)
%config(noreplace)%{_sysconfdir}/nginx/conf.d/%name.conf

%files api
%defattr(0660,%{webuser},%{webgroup})
%dir %attr(2770,%{webuser},%{webgroup}) %{webroot}
%dir %{webroot}/api
%{webroot}/api/*
%{webroot}/api/.htaccess

%files ui
%defattr(0660,%{webuser},%{webgroup})
%dir %{webroot}/ui
%{webroot}/ui/index.html
%{webroot}/ui/css
%{webroot}/ui/dist
%{webroot}/ui/fonts
%{webroot}/ui/img
%{webroot}/ui/js
%{webroot}/ui/lib
%{webroot}/ui/share
%dir %attr(2770,%{webuser},%{webgroup}) %{webroot}/ui/store
%dir %attr(2770,%{webuser},%{webgroup}) %{webroot}/ui/store/dashboard
%dir %attr(2770,%{webuser},%{webgroup}) %{webroot}/ui/store/profile
%{webroot}/ui/store/index.html
%{webroot}/ui/store/dashboard/index.html
%{webroot}/ui/store/dashboard/*.json
%{webroot}/ui/store/profile/index.html
%{webroot}/ui/templates

%prep
%setup -b0 -q

%install
# documantation
%{__mkdir} -p %{buildroot}%{_docdir}/%name
%{__install} -D -m 644 ui/COPYING %{buildroot}%{_docdir}/%name/COPYING
%{__install} -D -m 644 README.md %{buildroot}%{_docdir}/%name/README
# MySQL schemas, scripts and configs
%{__mkdir} -p %{buildroot}%{_datadir}/%name/sql
%{__cp} api/sql/* %{buildroot}%{_datadir}/%name/sql
%{__mkdir} -p %{buildroot}%{_bindir}
%{__mkdir} -p %{buildroot}%{_sysconfdir}/cron.daily
%{__cp} api/scripts/new_table.pl %{buildroot}%{_bindir}/homer_mysql_new_table.pl
%{__cp} api/scripts/partrotate_unixtimestamp.pl %{buildroot}%{_bindir}/homer_mysql_partrotate_unixtimestamp.pl
%{__cp} api/scripts/rotate.sh %{buildroot}%{_bindir}/homer_mysql_rotate
%{__ln_s} -f %{_bindir}/homer_mysql_rotate %{buildroot}%{_sysconfdir}/cron.daily/homer_mysql_logrotate
# http-server config directories and files
%{__mkdir} -p %{buildroot}%{_sysconfdir}/httpd/conf.d
%{__mkdir} -p %{buildroot}%{_sysconfdir}/nginx/conf.d
%{__install} -D -m 644 api/examples/web/%name.conf.httpd %{buildroot}%{_sysconfdir}/httpd/conf.d/%name.conf
%{__install} -D -m 644 api/examples/web/%{name}5.nginx %{buildroot}%{_sysconfdir}/nginx/conf.d/%name.conf
# Kamailio in sipcapture role config directories and files
%{__mkdir} -p %{buildroot}%{_sysconfdir}/systemd/system/multi-user.target.wants
%{__mkdir} -p %{buildroot}%{_localstatedir}/run/sipcapture
%{__install} -D -m644 api/examples/sipcapture/sipcapture.sysconfig %{buildroot}%{_sysconfdir}/sysconfig/sipcapture
%{__install} -D -m644 api/examples/sipcapture/sipcapture.service %{buildroot}%{_unitdir}/sipcapture.service
%{__ln_s} -f %{_unitdir}/sipcapture.service %{buildroot}%{_sysconfdir}/systemd/system/multi-user.target.wants/sipcapture.service
%{__install} -D -m644 api/examples/sipcapture/kamailio.cfg %{buildroot}%{_sysconfdir}/kamailio/kamailio-sipcapture.cfg
%{__install} -D -m644 api/examples/sipcapture/sipcapture.rsyslogd %{buildroot}%{_sysconfdir}/rsyslog.d/sipcapture.conf
%{__install} -D -m644 api/examples/sipcapture/sipcapture.logrotated %{buildroot}%{_sysconfdir}/logrotate.d/sipcapture
# UI and API directories and files
%{__mkdir} -p %{buildroot}%{webroot}/api
%{__mkdir} -p %{buildroot}%{webroot}/ui
%{__cp} -r api/api/Authentication %{buildroot}%{webroot}/api
%{__cp} -r api/api/Database %{buildroot}%{webroot}/api
%{__cp} -r api/api/RestApi %{buildroot}%{webroot}/api
%{__cp} -r api/api/RestService %{buildroot}%{webroot}/api
%{__cp} -r api/api/Statistic %{buildroot}%{webroot}/api
%{__cp} -r api/api/*.php %{buildroot}%{webroot}/api
%{__cp} -r api/api/.htaccess %{buildroot}%{webroot}/api
%{__cp} -r ui/index.html %{buildroot}%{webroot}/ui/index.html
%{__cp} -r ui/css %{buildroot}%{webroot}/ui
%{__cp} -r ui/dist %{buildroot}%{webroot}/ui
%{__cp} -r ui/fonts %{buildroot}%{webroot}/ui
%{__cp} -r ui/img %{buildroot}%{webroot}/ui
%{__cp} -r ui/js %{buildroot}%{webroot}/ui
%{__cp} -r ui/lib %{buildroot}%{webroot}/ui
%{__cp} -r ui/share %{buildroot}%{webroot}/ui
%{__cp} -r ui/store %{buildroot}%{webroot}/ui
%{__cp} -r ui/templates %{buildroot}%{webroot}/ui

%post

systemctl daemon-reload
systemctl restart rsyslog

%preun

systemctl stop sipcapture

%postun

systemctl daemon-reload
systemctl restart rsyslog

