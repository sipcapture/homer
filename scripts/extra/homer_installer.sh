#!/bin/sh
#
# --------------------------------------------------------------------------------
# HOMER/SipCapture automated installation script for Debian/CentOs/OpenSUSE (BETA)
# --------------------------------------------------------------------------------
# This script is only intended as a quickstart to test and get familiar with HOMER.
# It is not suitable for high-traffic nodes, complex capture scenarios, clusters.
# The HOW-TO should be ALWAYS followed for a fully controlled, manual installation!
# --------------------------------------------------------------------------------
#
#  Copyright notice:
#
#  (c) 2011-2013 Lorenzo Mangani <lorenzo.mangani@gmail.com>
#  (c) 2011-2013 Alexandr Dubovikov <alexandr.dubovikov@gmail.com>
#
#  All rights reserved
#
#  This script is part of the HOMER project (http://sipcapture.org)
#  The HOMER project is free software; you can redistribute it and/or 
#  modify it under the terms of the GNU Affero General Public License as 
#  published by the Free Software Foundation; either version 3 of 
#  the License, or (at your option) any later version.
#
#  You should have received a copy of the GNU Affero General Public License
#  along with this program.  If not, see <http://www.gnu.org/licenses/>
#
#  This script is distributed in the hope that it will be useful,
#  but WITHOUT ANY WARRANTY; without even the implied warranty of
#  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
#  GNU Affero General Public License for more details.
#
#  This copyright notice MUST APPEAR in all copies of the script!
#


VERSION=0.6.5
HOSTNAME=$(hostname)

clear; 
echo "*************************************************************"
echo "                                                             "
echo "      ,;;;;,       HOMER SIP CAPTURE (http://sipcapture.org) "
echo "     ;;;;;;;;.     Single-Node Auto-Installer (beta $VERSION)"
echo "   ;;;;;;;;;;;;                                              "
echo "  ;;;;  ;;  ;;;;   <--------------- INVITE ---------------   "
echo "  ;;;;  ;;  ;;;;    --------------- 200 OK --------------->  "
echo "  ;;;;  ..  ;;;;                                             " 
echo "  ;;;;      ;;;;   WARNING: This installer is intended for   "
echo "  ;;;;  ;;  ;;;;   dedicated/vanilla OS setups without any   "
echo "  ,;;;  ;;  ;;;;   customization and with default settings   "
echo "   ;;;;;;;;;;;;                                              "
echo "    :;;;;;;;;;     THIS SCRIPT IS PROVIDED AS-IS, USE AT     "
echo "     ;;;;;;;;      YOUR *OWN* RISK, REVIEW LICENSE & DOCS    "
echo "                                                             "
echo "*************************************************************"
echo;

# Check if we're good on permissions
if  [ "$(id -u)" != "0" ]; then
  echo "ERROR: You must be a root user. Exiting..." 2>&1
  echo  2>&1
  exit 1
fi

PATH=/bin:/sbin:/usr/bin:/usr/sbin:/usr/local/bin:/usr/local/sbin

echo "OS: Dectecting System...."
# Identify Linux Flavour
if [ -f /etc/debian_version ] ; then
    DIST="DEBIAN"
    echo "OS: DEBIAN detected"
elif [ -f /etc/redhat-release ] ; then
    DIST="CENTOS"
    echo "OS: CENTOS detected"
elif [ -f /etc/SuSE-release ] ; then
   DIST="SUSE"
   echo "OS: SUSE detected"
else
    echo "ERROR:"
    echo "Sorry, this Installer supports Debian, CentOS, SUSE based systems only!"
    echo "Please follow instructions in the HOW-TO for manual installation & setup"
    echo "available at http://sipcapture.org or http://homer.googlecode.com"
    echo
    exit 1
fi


# Identify Architecture
BITS=$(getconf LONG_BIT)
case $BITS in
    '32')
   echo "OS: 32bit detected"
   ;;
    '64')
   echo "OS: 64bit detected"
   ;;
esac


func_netcheck() {
# Check connectivity
hash wget 2>&- || { echo >&2 "I require wget but it's not installed."; };
wget -q --tries=10 --timeout=5 'http://www.sipcapture.org/installer.html' -O /tmp/net.check &> /dev/null 
if [ ! -s /tmp/net.check ]; then 
    echo "WARNING:"
    echo "This installer requires internet connectivity to proceed further successfully."
    echo "Check or adjust your settings temporarly. If your setup does not allow this,"
    echo "please follow instructions in the HOW-TO for manual installation & setup"
    echo "available at http://sipcapture.org or http://homer.googlecode.com"
    echo
    #exit 1
else
    echo "OS: Internet connectivity up"
    echo
fi
    rm -rf /tmp/net.check
}

     # We should be good to go!
     echo
     echo "Initializing... Ready!"
     echo 

# Check if Install Directory Present
if [ ! $1 ] || [ -z "$1" ] ; then
     # Prompt for installation directory or do default
     echo "This script will download, compile and install the requirements for Homer automatically."
     echo
     echo "Enter HOMER/Kamailio install path, press enter for default or 'abort' [/usr/local/kamailio]: "
     read  destination
     [ "$destination" = "abort" ] && return
     [ "$destination" = "" ] && destination="/usr/local/kamailio"
else
     destination=$1
fi

echo "HOMER/Kamailio will be installed to '$destination'"
echo
# Set full path, check if local
SETUP_ENV=$destination
echo "$SETUP_ENV" |grep '^/' -q && REAL_PATH=$SETUP_ENV || REAL_PATH=$PWD/$SETUP_ENV



# Setup Kamailio-DEV w/ sipcapture module from GIT
echo
echo "**************************************************************"
echo " INSTALLING OS PACKAGES AND DEPENDENCIES FOR HOMER SIPCAPTURE"
echo "**************************************************************"
echo
echo "This might take a while depending on system/network speed. Please stand by...."
echo

case $DIST in
    'DEBIAN')
   WEBROOT="/var/www/"
   WEBSERV="apache2"
   MYSQL="mysql"
   # Is this required?
        #apt-get -y update
        apt-get -y install autoconf automake autotools-dev binutils bison build-essential cpp curl flex g++ gcc git-core libxml2 libxml2-dev lynx m4 make mcrypt ncftp nmap openssl sox ssl-cert ssl-cert unzip zip zlib1g-dev zlib1g-dev libjpeg-dev sox mysql-server mysql-client libmysqlclient15-dev libcurl4-openssl-dev apache2 php5 php5-cli php5-gd php-pear php5-dev php5-mysql php-services-json
        pear channel-update pear.php.net
        pear install pecl/json
        ;;
    'SUSE')
   WEBROOT="/srv/www/htdocs/"
   WEBSERV="apache2"
   MYSQL="mysql"
   yast2 -i gcc make git bison flex apache2 apache2-mod_php5 libmysqlclient-devel libmysqlclient18 libmysqlclient_r18 mysql-community-server mysql-community-server-client php5-mysql php5-bcmath php5-bz2 php5-calendar php5-ctype php5-curl php5-dom php5-ftp php5-gd php5-gettext php5-gmp php5-iconv php5-imap php5-ldap php5-mbstring php5-mcrypt php5-odbc php5-openssl php5-pcntl php5-pgsql php5-posix php5-shmop php5-snmp php5-soap php5-sockets php5-sqlite php5-sysvsem php5-tokenizer php5-zlib php5-exif php5-fastcgi php5-pear php5-sysvmsg php5-sysvshm
   chkconfig mysql on
   chkconfig apache2 on
   /etc/init.d/mysql start
   mysql_secure_installation
   /etc/init.d/apache2 start
   ;;
    'CENTOS')
   WEBROOT="/var/www/html/"
   WEBSERV="httpd"
   MYSQL="mysqld"

   # Is this required?
        #yum -y update

        VERS=$(cat /etc/redhat-release |cut -d' ' -f3 |cut -d'.' -f1)
   cat /etc/redhat-release | grep -e "5." -e "5.6" -e "5.7" -q >> /dev/null
   if [ $? == "0" ]; then
   	yum -y remove php-*
           PHPV="53"
   	PHPJ="php53-common"
   else
   	PHPV=""
   	PHPJ="php-json"
   fi

        COMMON_PKGS=" autoconf automake bzip2 cpio curl curl-devel curl-devel expat-devel fileutils make gcc gcc-c++ gettext-devel gnutls-devel openssl openssl-devel openssl-devel perl patch unzip wget zip zlib zlib-devel bison flex mysql mysql-server mysql-devel pcre-devel libxml2-devel sox httpd php$PHPV php$PHPV-gd php$PHPV-mysql $PHPJ"
        if [ "$VERS" = "6" ]
        then
            yum -y install $COMMON_PKGS git
       if [ ! "$?" == "0" ]; then
   	echo 
   	echo "HALT! Something went wrong. Please resolve the errors above and try again."
   	exit 1
       fi

        else
            yum -y install $COMMON_PKGS
       if [ ! "$?" == "0" ]; then
   	echo 
   	echo "HALT! Something went wrong. Please resolve the errors above and try again."
   	exit 1
       fi
            #install the RPMFORGE Repository
            if [ ! -f /etc/yum.repos.d/rpmforge.repo ]
            then
                # Install RPMFORGE Repo
                rpm --import http://apt.sw.be/RPM-GPG-KEY.dag.txt
echo '
[rpmforge]
name = Red Hat Enterprise $releasever - RPMforge.net - dag
mirrorlist = http://apt.sw.be/redhat/el5/en/mirrors-rpmforge
enabled = 0
protect = 0
gpgkey = file:///etc/pki/rpm-gpg/RPM-GPG-KEY-rpmforge-dag
gpgcheck = 1
' > /etc/yum.repos.d/rpmforge.repo
            fi
            yum -y --enablerepo=rpmforge install git-core
       if [ ! "$?" == "0" ]; then
   	echo 
   	echo "HALT! Something went wrong. Please resolve the errors above and try again."
   	exit 1
       fi

        fi
        ;;
esac

# General Optimizations and random extras
if [ "$DIST" == "DEBIAN" ]; then
   # Find and check suhosin.ini for get_max_vars
   suhosin=$(find / -name suhosin.ini)
   if [ "$suhosin" ] ; then
   	insuhosin=$(cat "$suhosin" | grep "suhosin.get.max_vars" )
   	if [ $insuhosin == ;* ]; then
   		echo "suhosin.get.max_vars = 500" >> "$suhosin"
   	else
   		orig="$insuhosin"
   		dest="suhosin.get.max_vars = 500"
   		sed -i -e "s#$orig#$dest#g" $suhosin
   	fi
   fi
fi

# Prompt for DB credentials
func_dbauth() {
echo
case $DIST in
    'DEBIAN')
   echo "Enter the next details based on the parameters entered during MySQL setup"
   echo "MYSQL User: "
   read sqlusername
   echo "MYSQL Pass: "
   stty -echo
   read sqlpassword
   stty echo
   ;;
    'SUSE')
   echo "Enter the next details based on the parameters entered during MySQL setup"
   echo "MYSQL ROOT User: "
   read sqlusername
   echo "MYSQL ROOT Pass: "
   stty -echo
   read sqlpassword
   stty echo
   #myqladmin -u root password $sqlpassword
   #mysql_secure_installation
   ;;
    'CENTOS')
   #read -p "DB User: " sqlusername
   if [ ! -f "/etc/init.d/$MYSQL" ]; then
   	echo
   	echo "HALT! There is no database. Please install $MYSQL manually and retry."
   	exit 1
   fi
   /etc/init.d/$MYSQL start
   read -p "MYSQL ROOT Pass: " sqlpassword
   sqlusername=root
   # check
   DBEXEC=$(mysqladmin -u root password $sqlpassword 2>&1 | grep error)
   if [ ! "$DBEXEC" == "" ]; then
   	# check if already setup
   	echo "Existing password! Testing..."
   	DBEXEC=$(mysqladmin -u root -p$sqlpassword password $sqlpassword 2>&1 | grep error)
   		if [ "$DBEXEC" == ""  ]; then
   			echo "Database Access: OK"
   		else
   			echo "Database Access: FAILED"
   			echo
   			echo "ERROR: Database Access was denied. Please check your access credentials and try again"
   			echo "-------------------------------------------------------------------------------------"
   			echo
   			func_dbauth;
   		fi

   fi
   ;;
esac

   if [ "$sqlusername" = "" ] ; then
   	sqlusername="root"
   fi
}

# Execute DB Auth Function
echo; echo;
echo "**************************************************************"
echo " MYSQL - PROVIDE ROOT ACCESS TO SETUP TABLES REQUIRED BY HOMER"
echo "**************************************************************"
echo ""
func_dbauth;


# Setup Kamailio/Sipcapture from GIT
echo; echo;
echo "**************************************************************"
echo " SIPCAPTURE/KAMAILIO - INSTALLING LATEST VERSION FROM GIT REPO"
echo "**************************************************************"
echo
if [ ! -d "/usr/src/kamailio-devel" ]; then
   mkdir -p /usr/src/kamailio-devel
   cd /usr/src/kamailio-devel
   echo "GIT: Cloning Kamailio..."
   git clone --depth 1 git://git.sip-router.org/sip-router kamailio
   cd kamailio
else
   echo "GIT: Updating Kamailio..."
   cd /usr/src/kamailio-devel/kamailio
   git pull
fi

if [ -z "$REAL_PATH" ]; then
 REAL_PATH = "/usr/local/kamailio"
fi

echo "Configuring Kamailio configuration flavour & enabling modules"
make PREFIX=$REAL_PATH FLAVOUR=kamailio include_modules="db_mysql sipcapture" cfg
echo 
echo "This might take a while. Please stand by..."
echo
   make all && make install

# Place example configuration in place
mv $REAL_PATH/etc/kamailio/kamailio.cfg $REAL_PATH/etc/kamailio/kamailio.cfg.old
cp modules/sipcapture/examples/kamailio.cfg $REAL_PATH/etc/kamailio/kamailio.cfg

# END: KAMAILIO

# START: MYSQL
# Setup database
/etc/init.d/$MYSQL restart


echo
echo "Settings system user/group/permissions for Kamailio..."
echo

# Add kamailio user and group and set permissions
# TODO: check if already existing
if [ ! -d "/var/run/kamailio" ]; then
   mkdir /var/run/kamailio
fi

if [ "$DIST" == "DEBIAN" ]; then
   adduser --group --system --home /var/run/kamailio --disabled-password -gecos "Kamailio" --shell /bin/false kamailio
   chown kamailio:kamailio /var/run/kamailio
elif  [ "$DIST" == "SUSE" ]; then
   useradd -r -d /var/run/kamailio -U 022 -s /sbin/nologin kamailio
   chown kamailio /var/run/kamailio
else
   adduser -r -d /var/run/kamailio --shell /sbin/nologin kamailio
   iptables -A INPUT -p tcp --dport 80 -j ACCEPT
   iptables-save
   chown kamailio:kamailio /var/run/kamailio
fi



# START: WEBHOMER
# Setup Kamailio/Sipcapture from GIT
echo
echo "**************************************************************"
echo " SIPCAPTURE/WEBHOMER: INSTALLING LATEST VERSION FROM GIT REPO"
echo "**************************************************************"
echo
mkdir -p /usr/src/homer-git
cd /usr/src/homer-git
if [ ! -d "/usr/src/homer-git/homer" ]; then
   echo "GIT: Cloning webHomer..."
   git clone https://code.google.com/p/homer/
   cd homer
else
   echo "GIT: Updating webHomer..."
   cd homer
   git pull
fi
   # Define database options
   echo "HOMER TABLES SETUP..."
   echo "Choose MYSQL Homer User (blank for 'homer') : "
   read sqlhomeruser
   echo "Choose MYSQL Homer Pass (blank for random) : "
   stty -echo
   read sqlhomerpassword
   stty echo
   if [ "$sqlhomeruser" = "" ] ; then
   	echo "Using default username..."
   	sqlhomeruser="homer"
   fi
   if [ "$sqlhomerpassword" = "" ] ; then
   	echo "Using random password... "
   	sqlhomerpassword=$(cat /dev/urandom|tr -dc "a-zA-Z0-9-_\$\?"|fold -w 9|head -n 1)
   fi

   # Create MySQL Databases & Import schemas
   dbcheck1=$(mysql -u "$sqluser" -p"$sqlpassword" --batch --skip-column-names -e "SHOW DATABASES LIKE 'homer_db'")
   if [ -z "$dbcheck1" ] ; then
   	echo "Creating Databases..."
   	mysql -u "$sqluser" -p"$sqlpassword" -e "create database IF NOT EXISTS homer_db;";
   	mysql -u "$sqluser" -p"$sqlpassword" -e "create database IF NOT EXISTS homer_users;";
   	echo "Creating Users..."
   	mysql -u "$sqluser" -p"$sqlpassword" -e "GRANT ALL ON *.* TO '$sqlhomeruser'@'localhost' IDENTIFIED BY '$sqlhomerpassword'; FLUSH PRIVILEGES;";
   	echo "Creating Tables..."
   	mysql -u "$sqluser" -p"$sqlpassword" homer_db < sql/create_sipcapture_version_3.sql
   	mysql -u "$sqluser" -p"$sqlpassword" homer_db < webhomer/sql/statistics.sql
   	mysql -u "$sqluser" -p"$sqlpassword" homer_users < webhomer/sql/homer_users.sql
   	mysql -u "$sqluser" -p"$sqlpassword" homer_users -e "TRUNCATE TABLE homer_nodes;"
     mysql -u "$sqluser" -p"$sqlpassword" homer_users -e "INSERT INTO homer_nodes VALUES(1, '127.0.0.1','homer_db','3306','"$sqluser"','"$sqlpassword"','node1', 1);"
   else
   	echo 
   	echo "WARNING: Existing/Conflicting database found!"
   	echo "         You MUST create your tables manually"
   	echo "         ------------------------------------"
   	echo 
   fi

if [ -d "$WEBROOT/webhomer" ]; then
   	echo
   	echo "WARNING: Existing webhomer found! Skipping..."
   	echo "         You MUST configure settings manually"
   	echo "         ------------------------------------"
   	echo 
else
   # Sync actual web folder with GIT webhomer
   cp -R webhomer $WEBROOT/
   # Fork example configuration & preferences
   mv $WEBROOT/webhomer/configuration_example.php $WEBROOT/webhomer/configuration.php
   mv $WEBROOT/webhomer/preferences_example.php $WEBROOT/webhomer/preferences.php

   chmod 777 "$WEBROOT/webhomer/tmp"
   echo "Forking and patching webHomer configuration..."

   # Define DB Access Credentials
   orig="'USER', \"root\""
   dest="'USER', \"$sqlhomeruser\""
   sed -i -e "s#$orig#$dest#g" $WEBROOT/webhomer/configuration.php

   orig="'PW', \"root\""
   dest="'PW', \"$sqlhomerpassword\""
   sed -i -e "s#$orig#$dest#g" $WEBROOT/webhomer/configuration.php

   orig="'HOMER_USER', \"homer_user\""
   dest="'HOMER_USER', \"$sqlhomeruser\""
   sed -i -e "s#$orig#$dest#g" $WEBROOT/webhomer/configuration.php

   orig="'HOMER_PW', \"homer_password\""
   dest="'HOMER_PW', \"$sqlhomerpassword\""
   sed -i -e "s#$orig#$dest#g" $WEBROOT/webhomer/configuration.php

   # Adjust webroot folder location for paths in configuration.php
   # sed -i -e "s/http:\/\/localhost//g" $WEBROOT/webhomer/configuration.php

   # Ugly ways but still ways 
   if [ "$DIST" == "CENTOS" ]; then
   	sed -i -e "s/\/var\/www/\/var\/www\/html/g" $WEBROOT/webhomer/configuration.php
   elif [ "$DIST" == "SUSE" ]; then
   	sed -i -e "s/\/var\/www/\/srv\/www\/htdocs/g" $WEBROOT/webhomer/configuration.php
   fi

fi


# Copy maintenance scripts in place
if [ -f "/opt/partrotate_unixtimestamp.pl" ]; then
   	echo
   	echo "WARNING: Existing HOMER Scripts found!"
   	echo "         You MUST configure crontab manually"
   	echo "         -----------------------------------"
   	echo 

else
   echo "**************************************************************"
   echo " WEBHOMER/SIPCAPTURE: INSTALLING SCRIPTS & SCHEDULING CRONTAB "
   echo "**************************************************************"
   echo

   cp scripts/partrotate_unixtimestamp.pl /opt/
   # patch db credentials in script
        orig="$mysql_login = \"mysql_login\""
        dest="$mysql_login = \"$sqlhomeruser\""
        sed -i -e "s#$orig#$dest#g" /opt/partrotate_unixtimestamp.pl
        orig="$mysql_password = \"mysql_password\""
        dest="$mysql_password = \"$sqlhomerpassword\""
        sed -i -e "s#$orig#$dest#g" /opt/partrotate_unixtimestamp.pl
   # set permissions
   chmod 755 /opt/partrotate_unixtimestamp.pl
   chmod +x /opt/partrotate_unixtimestamp.pl

   cp scripts/statistic.pl /opt/
   # patch db credentials in script
   orig="$mysql_user = \"homer_user\""
        dest="$mysql_user = \"$sqlhomeruser\""
        sed -i -e "s#$orig#$dest#g" /opt/statistic.pl
   orig="$mysql_password = \"homer_password\""
        dest="$mysql_password = \"$sqlhomerpassword\""
        sed -i -e "s#$orig#$dest#g" /opt/statistic.pl
   # set permissions
   chmod 755 /opt/statistic.pl
   chmod +x /opt/statistic.pl

   # Set Cron: Statistic
   echo "" > /opt/homer.cron
   read -p "Install Statistics Cronjob? (y/N): " choice
   case "$choice" in 
     y|Y ) 
   	echo "Adding cronjob..."
   	statistic="/opt/statistic.pl 2>&1> /dev/null"
   	job1="5 * * * * sudo $statistic"
   	crontab -l > /opt/cron.tmp
   	echo "$job1" >> /opt/cron.tmp
   	CRON=$(cat /opt/cron.tmp | crontab -)
   	rm -rf /opt/cron.tmp
   	;;
     n|N|* ) echo "skipping";;
   esac

   # Set Cron: Statistic
   read -p "Install Rotation Cronjob? (y/N): " choice
   case "$choice" in
     y|Y )
           echo "Adding cronjob..."
   	# Set Cron: Partition Rotation 
   	rotate="/opt/partrotate_unixtimestamp.pl 2>&1> /dev/null"
   	job2="* 0 * * * sudo $rotate"
   	crontab -l > /opt/cron.tmp
   	echo "$job2" >> /opt/cron.tmp
   	CRON=$(cat /opt/cron.tmp | crontab - )
   	rm -rf /opt/cron.tmp
   	;;
     n|N|* ) echo "skipping";;
   esac


fi

# Restart HTTPD/APACHE2
echo "Restarting services..." 
/etc/init.d/$WEBSERV restart

# START: HOMER CONFIGURATION
echo "**************************************************************"
echo "HOMER/SIPCAPTURE: KAMAILIO CONFIGURATION & PATH EXTENSION  "
echo "**************************************************************"
echo

# WORK IN PROGRESS: Kamailio basic configurator
#
proc_kamsetup() {

# generate a full kamailio.cfg

config="$REAL_PATH/etc/kamailio/kamailio.new"
echo "Generating kamailio.cfg for Homer SIP Capture..."
echo
echo '
#!KAMAILIO
#
####### Global Parameters #########
## Generated by Homer Installer
## Alpha Version $VERSION 

debug=1
log_stderror=no
memdbg=5
memlog=5
log_facility=LOG_LOCAL0

fork=yes
children=5
/* uncomment the next line to disable TCP (default on) */
disable_tcp=yes

' > $config

case $BITS in
    '32')
   echo "mpath=\"/usr/local/lib/kamailio/modules_k/:/usr/local/lib/kamailio/modules/:/usr/local/kamailio/lib/kamailio/modules/:/usr/local/kamailio/lib/kamailio/modules_k/\"" >> $config
   ;;
    '64')
   echo "mpath=\"/usr/local/lib64/kamailio/modules_k/:/usr/local/lib64/kamailio/modules/:/usr/local/kamailio/lib64/kamailio/modules/:/usr/local/kamailio/lib64/kamailio/modules_k/\"" >> $config
   ;;
esac

echo '
loadmodule "db_mysql.so"
loadmodule "sipcapture.so"

# ----- mi_fifo params -----

####### Routing Logic ########
' >> $config
echo "modparam(\"sipcapture\", \"db_url\", \"mysql://$sqlhomeruser:$sqlhomerpassword@localhost/homer_db\")" >> $config
echo '
modparam("sipcapture", "capture_on", 1)
/* My node name*/
modparam("sipcapture", "capture_node", "node1")
/* My table name*/
modparam("sipcapture", "table_name", "sip_capture")
/* children for raw socket */
modparam("sipcapture", "raw_sock_children", 6)
' >> $config

# Local IP/PORT candidates
#read -p "Please confirm the local network device: [eth0]: " device
 if [ "$device" = "" ] ; then
   device="eth0"
 fi
   localip=$(ip addr show dev $device  | grep inet -w | awk '{ print $2 }' | sed s#/.*##g )
   if [ -z "$localip" ] || [ $localip == "" ]; then 
   	localip="127.0.0.1" 
   fi
   	localport="5060"

   echo
   echo "We are now going to configure the capture options:"
   echo
   echo "       1: HEP encapsulation socket (suggested)"
   echo "       2: IPIP encapsulation socket"
   echo "       3: RAW Port Monitoring/Mirroring"
   echo "       *: Manual Configuration (using HOWTO & FAQ)"
   echo 

   # Set choice
        read -p "Enter your choice or press enter to skip [*] : " choice
        case "$choice" in
          1 )
   	echo
   	read -p "HEP Capture IP [$localip] : " capip
    		if [ "$capip" = "" ] ; then
   			capip="$localip"
   		 fi
   	read -p "HEP Capture PORT [$localport] : " capport
    		if [ "$capport" = "" ] ; then
   			capport="$localport"
   		 fi
   	echo
# ready to go
echo "/* IP and port for HEP capturing) */" >> $config
echo "listen=udp:$capip:$capport" >> $config
echo '
/* activate HEP capturing */
modparam("sipcapture", "hep_capture_on", 1)
' >> $config
echo "NEXT: Configure a remote HEP Capture Agent (captagent OR integrated in FreeSWITCH, Kamailio, OpenSIPS)"
              ;;
          2 )
   	echo
echo '
/* Name of interface to bind on raw socket */
modparam("sipcapture", "raw_interface", "eth0")
/* activate IPIP capturing */
modparam("sipcapture", "raw_ipip_capture_on", 1)
' >> $config
echo "NEXT: Configure a remote IPIP Capture Agent (ACME Packet SBC, HAUWEI SBC, etc)"
              ;;
          3 )
   	echo
   	read -p "RAW Capture IP [$localip] : " capip
    		if [ "$capip" = "" ] ; then
   			capip="$localip"
   		 fi
   	read -p "RAW Capture PORT [$localport] : " capport
    		if [ "$capport" = "" ] ; then
   			capport="$localport"
   		 fi
   	echo

echo '
/* Name of interface to bind on raw socket */
modparam("sipcapture", "raw_interface", "eth0")
/* activate monitoring/mirroring port capturing. Linux only */
modparam("sipcapture", "raw_moni_capture_on", 1)
/* Promiscious mode RAW socket. Mirroring port. Linux only */
modparam("sipcapture", "promiscious_on", 1)
/* IP to listen. Port/Portrange apply only on mirroring port capturing */
' >> $config
echo "modparam(\"sipcapture\", \"raw_socket_listen\", \"$capip:$capport\")" >> $config

#echo "/* IP and port for HEP */" >> $config
#echo "/* listen=udp:$capip:$capport */" >> $config

# Old schema fix
# echo 'modparam("sipcapture", "authorization_column", "authorization")' >> $config

echo "NEXT: Configure switch to mirror/monitor all desired traffic to our port connected to $device"
                ;;
          * ) 
   	echo "Skipping capture configuration."
   	echo "Please configure your settings in $REAL_PATH/etc/kamailio/kamailio.cfg"
   	echo
   	;;
        esac
if [ -f "$config" ]; then
echo '

# Main SIP request routing logic
# - processing of any incoming SIP request starts with this route
route {
   #For example, you can capture only needed methods...
   #if (is_method("INVITE|UPDATE|NOTIFY|SUBSCRIBE|OPTIONS|REGISTER|BYE")) {
   	sip_capture();
   #}
   drop;
}

onreply_route {
   #And replies of request methods
   #if(status =~ "^(1[0-9][0-9]|[3[0-9][0-9]|4[0-9]|[56][0-9][0-9])$") {
   #if($rm =~ "^(INVITE|UPDATE|NOTIFY|SUBSCRIBE|OPTIONS|REGISTER|BYE)$") {
   	sip_capture();
   #}
   drop;
}

' >> $config

# set new configuration in place
   mv $config $REAL_PATH/etc/kamailio/kamailio.cfg
   echo "Kamailio configuration ready!"
   echo
# Check Kamailio config
   $REAL_PATH/sbin/kamailio -c

# Start Kamilio/SipCapture
   echo
   read -p "Would you like to start Kamailio/Sipcapture? (y/N): " choice
   case "$choice" in 
     y|Y ) 
   	$REAL_PATH/sbin/kamctl start
   	;;
     n|N|* ) 
   	echo "Start manually using:"
   	echo "$REAL_PATH/sbin/kamctl start"
   	echo
   	;;
   esac
else
   echo "Configuration failed."
   echo "Please configure your settings in $REAL_PATH/etc/kamailio/kamailio.cfg"
fi 
 }

   # Setup Kamailio capture?
   echo
   read -p "Would you like to configure SIP Capture options? (y/N): " choice
   case "$choice" in 
     y|Y ) 
   	proc_kamsetup;
   	;;
     n|N|* ) 
   	echo "skipping..."
   	echo "Please configure your settings in $REAL_PATH/etc/kamailio/kamailio.cfg"
   	;;
   esac


# Install Complete
clear
echo
echo "**************************************************************"
echo
echo " Congratulations! HOMER (kamailio+sipcapture) is now installed"
echo
echo "**************************************************************"
echo
echo " Your system is ready to run (but NOT yet running!)"
echo " Please complete the installation as follows:"
echo
echo "     * Verify configuration for WebHomer:"
echo "         '$WEBROOT/webhomer/configuration.php'"
echo "         '$WEBROOT/webhomer/preferences.php'"
echo
echo "     * Verify capture settings for Homer/Kamailio:"
echo "         '$REAL_PATH/etc/kamailio/kamailio.cfg'"
echo
echo "     * Start/stop Homer SIP Capture:"
echo "         '$REAL_PATH/sbin/kamctl start|stop'"
echo
echo "     * Access webHomer UI:"
echo "         http://$HOSTNAME/webhomer"
echo "         [default login: test@test.com/test123]"
echo
echo
echo "**************************************************************"
echo
echo " IMPORTANT: Do not forget to send Homer node some traffic! ;) "
echo " For more help and informations visit: http://sipcapture.org "
echo
echo "**************************************************************"
echo ""
exit 0
