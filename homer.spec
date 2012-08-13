Name:           homer
Version:        3.2.5
Release:        2%{?dist}
Summary:        SIP capture server

Group:          Applications/Communications
License:        GPLv3
URL:            http://www.sipcapture.org/
Source0:        %name-%version.tar.gz
BuildRoot:      %{_tmppath}/%{name}-%{version}-%{release}-root-%(%{__id_u} -n)

BuildRequires:  libpcap-devel
BuildRequires:  autoconf
BuildRequires:  automake
BuildRequires:  make
BuildRequires:  gcc
BuildRequires:  gcc-c++

%if %{_vendor} == suse

%define webroot %{_prefix}/srv/www/htdocs

Requires: apache2
Requires: apache2-mod_php5
Requires: libmysqlclient18
Requires: libmysqlclient_r18
# technically mysql can be on another server
Requires: mysql-community-server
Requires: mysql-community-server-client
Requires: php5-mysql
# These should be verified these, copied from homer_install.sh --Douglas
Requires: php5-bcmath
Requires: php5-bz2
Requires: php5-calendar
Requires: php5-ctype
Requires: php5-curl
Requires: php5-dom
Requires: php5-ftp
Requires: php5-gd
Requires: php5-gettext
Requires: php5-gmp
Requires: php5-iconv
Requires: php5-imap
Requires: php5-ldap
Requires: php5-mbstring
Requires: php5-mcrypt
Requires: php5-odbc
Requires: php5-openssl
Requires: php5-pcntl
Requires: php5-pgsql
Requires: php5-posix
Requires: php5-shmop
Requires: php5-snmp
Requires: php5-soap
Requires: php5-sockets
Requires: php5-sqlite
Requires: php5-sysvsem
Requires: php5-tokenizer
Requires: php5-zlib
Requires: php5-exif
Requires: php5-fastcgi
Requires: php5-pear
Requires: php5-sysvmsg
Requires: php5-sysvshm

%else # CentOS/Fedora

%define webroot %{_localstatedir}/www/html

%if 0%{?rhel} < 6 && 0%{?fedora} == 0
%define php php53
# part of json support is now in php-common
Requires: %{php}-json
%else
%define php php
%endif

Requires: mysql
# technically mysql can be on another server
Requires: mysql-server
Requires: httpd
Requires: %{php}
Requires: %{php}-gd
Requires: %{php}-mysql
%endif

%description
HOMER is a robust, carrier-grade, scalable SIP Capture system and Monitoring Application with HEP/HEP2, IP Proto4 (IPIP) encapsulation & port mirroring/monitoring support right out of the box, ready to process & store insane amounts of signaling with instant search, end-to-end analysis and drill-down capabilities for ITSPs, VoIP Providers and Trunk Suppliers using SIP signaling

%prep
%setup -q

%build
%configure
make %{?_smp_mflags}

%install
rm -rf $RPM_BUILD_ROOT
make install DESTDIR=$RPM_BUILD_ROOT

%clean
rm -rf $RPM_BUILD_ROOT

%files
%defattr(-,root,root,-)
%doc
%attr(755,root,root) %{_bindir}/captagent
%attr(755,root,root) %{_libexecdir}/*
%{_sysconfdir}/captagent/captagent.ini
%dir %attr(755,apache,apache) %{webroot}/webhomer/tmp
%dir %attr(755,apache,apache) %{webroot}/webhomer/tmp/colors
%{webroot}/webhomer/modules/*
%{webroot}/webhomer/php-sip/*
%{webroot}/webhomer/js/*
%{webroot}/webhomer/class/*
%{webroot}/webhomer/components/*
%{webroot}/webhomer/api/*
%{webroot}/webhomer/api/.htaccess
%{webroot}/webhomer/sql/*
%{webroot}/webhomer/styles/*
%{webroot}/webhomer/images/*
%{webroot}/webhomer/DataTable/*
%{webroot}/webhomer/*.php
%{webroot}/webhomer/*.ico
%{webroot}/webhomer/*.ttf

%changelog
* Fri Mar 9 2012 Douglas Hubler <douglas@hubler.us>
- Initial version.
