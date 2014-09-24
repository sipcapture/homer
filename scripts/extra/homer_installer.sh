#!/bin/bash
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
#  (c) 2011-2014 Lorenzo Mangani <lorenzo.mangani@gmail.com>
#  (c) 2011-2014 Alexandr Dubovikov <alexandr.dubovikov@gmail.com>
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

#####################################################################
#                                                                   #
#  WARNING: THIS SCRIPT IS NOW UPDATED TO SUPPORT HOMER 3.6+        #
#           PLEASE USE WITH CAUTION AND HELP US BY REPORTING BUGS!  #
#                                                                   #
#####################################################################

VERSION=0.7.4
HOSTNAME=$(hostname)

logfile=/tmp/homer_installer.log

# LOG INSTALLER OUTPUT TO $logfile
mkfifo ${logfile}.pipe
tee < ${logfile}.pipe $logfile &
exec &> ${logfile}.pipe
rm ${logfile}.pipe

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

MAINIF=$( route -n | grep '^0\.0\.0\.0' | head -n 1 | awk '{print $NF}' )
MAINIP=$( ifconfig $MAINIF | { IFS=' :';read r;read r r a r;echo $a; } )

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

        COMMON_PKGS=" autoconf automake bzip2 cpio curl curl-devel curl-devel expat-devel fileutils make gcc gcc-c++ gettext-devel gnutls-devel openssl openssl-devel openssl-devel mod_ssl perl patch unzip wget zip zlib zlib-devel bison flex mysql mysql-server mysql-devel pcre-devel libxml2-devel sox httpd php$PHPV php$PHPV-gd php$PHPV-mysql $PHPJ"
     if [ "$VERS" = "6" ]
        then
            yum -y install $COMMON_PKGS git php-mysql php-devel php-gd
            chkconfig mysqld on
            chkconfig httpd on
            /etc/init.d/mysqld start
            mysql_secure_installation
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


# TODO: Disable PHP notices (reported to cause issues in 3.5)
# .htaccess: php_value error_reporting 8191
#            php_flag display_errors off


# Prompt for DB credentials
func_dbauth() {
echo
case $DIST in
    'DEBIAN')
   echo "Enter the next details based on the parameters entered during MySQL setup (root)"
   echo "MYSQL User: "
   read sqlusername
   echo "MYSQL Pass: "
   stty -echo
   read sqlpassword
   stty echo
   ;;
    'SUSE')
   echo "Enter the next details based on the parameters entered during MySQL setup (root)"
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
   echo "Configuring Kamailio configuration flavour & enabling modules"
   make PREFIX=$REAL_PATH FLAVOUR=kamailio include_modules="db_mysql sipcapture pv textops rtimer xlog sqlops htable sl siputils" cfg
   echo 
   echo "This might take a while. Please stand by..."
   echo
   make all && make install
   # Place example configuration in place
   mv $REAL_PATH/etc/kamailio/kamailio.cfg $REAL_PATH/etc/kamailio/kamailio.cfg.old
   cp modules/sipcapture/examples/kamailio.cfg $REAL_PATH/etc/kamailio/kamailio.cfg
else
   echo
   read -p "Existing Kamailio! Would you like to update Kamailio/Sipcapture? (y/N): " choice
   case "$choice" in 
     y|Y) 
         echo "GIT: Updating Kamailio..."
         cd /usr/src/kamailio-devel/kamailio
         git pull
         echo "Configuring Kamailio configuration flavour & enabling modules"
         make PREFIX=$REAL_PATH FLAVOUR=kamailio include_modules="db_mysql sipcapture pv textops rtimer xlog sqlops htable sl siputils tm" cfg
         echo 
         echo "This might take a while. Please stand by..."
         echo
         make all && make install
         # Place example configuration in place
         mv $REAL_PATH/etc/kamailio/kamailio.cfg $REAL_PATH/etc/kamailio/kamailio.cfg.old
         cp modules/sipcapture/examples/kamailio.cfg $REAL_PATH/etc/kamailio/kamailio.cfg

        ;;
     n|N|*) 
        echo "Skipping update... (at your own risk)"
        echo
        ;;
   esac
   
fi

if [ -z "$REAL_PATH" ]; then
 REAL_PATH = "/usr/local/kamailio"
fi


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
   iptables -D INPUT -p tcp --dport 80 -j ACCEPT
   iptables -A INPUT -p tcp --dport 80 -j ACCEPT
   sudo iptables -I INPUT 4 -p tcp -m state --state NEW -m tcp --dport 80 -j ACCEPT
   sudo iptables -I INPUT 4 -p tcp -m state --state NEW -m tcp --dport 443 -j ACCEPT
   # iptables-save
   /sbin/service iptables save 
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
   	mysql -u "$sqluser" -p"$sqlpassword" homer_db < sql/create_sipcapture_version_4.sql
   	mysql -u "$sqluser" -p"$sqlpassword" homer_db < webhomer/sql/statistics.sql
   	mysql -u "$sqluser" -p"$sqlpassword" homer_users < webhomer/sql/homer_users.sql
   	mysql -u "$sqluser" -p"$sqlpassword" homer_users -e "TRUNCATE TABLE homer_nodes;"
     echo "Creating local DB Node..."
     mysql -u "$sqluser" -p"$sqlpassword" homer_users -e "INSERT INTO homer_nodes VALUES(1,'127.0.0.1','homer_db','3306','"$sqlhomeruser"','"$sqlhomerpassword"','sip_capture','node1', 1);"
   else
   	echo 
   	echo "WARNING: Existing/Conflicting database found!"
   	echo "         You MUST create your tables manually or remove the database!"
   	echo "         ------------------------------------------------------------"
   	echo 
   fi

if [ -d "$WEBROOT/webhomer" ]; then
   	echo
   	echo "WARNING: Existing webhomer found! Skipping..."
   	echo "         You MUST configure settings manually or remove the directory"
   	echo "         ------------------------------------------------------------"
   	echo 
else
   # Sync actual web folder with GIT webhomer
   cp -R webhomer $WEBROOT/
   # Fork example configuration & preferences
   mv $WEBROOT/webhomer/configuration_example.php $WEBROOT/webhomer/configuration.php
   mv $WEBROOT/webhomer/preferences_example.php $WEBROOT/webhomer/preferences.php

   chmod 777 "$WEBROOT/webhomer/tmp"
   echo "Forking and patching webHomer configuration..."
   
   # Enable auto-detection of ports
   orig="define('CFLOW_HPORT', 0);"
   dest="define('CFLOW_HPORT', 2);"
   sed -i -e "s#$orig#$dest#g" $WEBROOT/webhomer/preferences.php
   
   if  [ "$DIST" == "CENTOS" ]; then
     tzone=$(cat /etc/sysconfig/clock | awk -F '"' '{print $2}')
   else  
     tzone=$(cat /etc/timezone);
   fi

   if [ ! "$tzone" = "" ] ; then
        echo "Setting timezone to '$tzone'..."
         # Configure system Timezone
         orig="define('HOMER_TIMEZONE', \"America/Detroit\");"
         dest="define('HOMER_TIMEZONE', \"$tzone\");"
         sed -i -e "s#$orig#$dest#g" $WEBROOT/webhomer/preferences.php
   fi
   
   if  [ "$DIST" == "CENTOS" ]; then
     # Fix path for CentOS httpd
         orig="define('APILOC',\"/webhomer/api/\");"
         dest="define('APILOC',\"/api/\");"
         sed -i -e "s#$orig#$dest#g" $WEBROOT/webhomer/configuration.php
         
         orig="define('WEBPCAPLOC',\"/webhomer/tmp/\");"
         dest="define('WEBPCAPLOC',\"/tmp/\");"
         sed -i -e "s#$orig#$dest#g" $WEBROOT/webhomer/configuration.php
         
   fi
   

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

# Setup Kamailio init scripts?
   echo
   read -p "Would you like to install Kamailio's init/defaults scripts? (y/N): " choice
   case "$choice" in 
     y|Y ) 
        if [ "$DIST" == "DEBIAN" ]; then
           # INIT SCRIPTS
             #cp /usr/src/kamailio-devel/kamailio/pkg/kamailio/deb/debian/kamailio.init /etc/init.d/kamailio
             cp /usr/src/homer-git/homer/scripts/extra/kamailio/kamailio.debian.init /etc/init.d/kamailio
             chmod 755 /etc/init.d/kamailio 
           # Patch Init scripts - DAEMON
             #orig="DAEMON=/usr/sbin/kamailio"
             #dest="DAEMON=/usr/local/sbin/kamailio"
             #sed -i -e "s#$orig#$dest#g" /etc/init.d/kamailio
           # Patch Init scripts = CFGFILE
             #orig="CFGFILE=/etc/kamailio/kamailio.cfg"
             #dest="CFGFILE=$REAL_PATH/etc/kamailio/kamailio.cfg"
             #sed -i -e "s#$orig#$dest#g" /etc/init.d/kamailio
           # Patch Init scripts - RUN_KAMAILIO (needed?)
           #    orig="RUN_KAMAILIO=no"
           #    dest="RUN_KAMAILIO=yes"
           #    sed -i -e "s#$orig#$dest#g" /etc/init.d/kamailio
           # DEFAULTS
             #cp /usr/src/kamailio-devel/kamailio/pkg/kamailio/deb/debian/kamailio.default /etc/default/kamailio
           # Patch Init scripts - RUN_KAMAILIO
             #orig="#RUN_KAMAILIO=yes"
             #dest="RUN_KAMAILIO=yes"
             #sed -i -e "s#\$orig#$dest#g" /etc/default/kamailio

        elif  [ "$DIST" == "CENTOS" ]; then
           # INIT SCRIPTS
             #cp /usr/src/kamailio-devel/kamailio/pkg/kamailio/rpm/kamailio.init /etc/init.d/kamailio
             cp /usr/src/kamailio-devel/kamailio/pkg/kamailio/rpm/kamailio.init /etc/init.d/kamailio
             chmod 755 /etc/init.d/kamailio
             cp /usr/src/kamailio-devel/kamailio/pkg/kamailio/rpm/kamailio.default /etc/default/kamailio

             # Patch Init scripts - DAEMON
             orig="KAM=/usr/sbin/kamailio"
             dest="KAM=$REAL_PATH/sbin/kamailio"
             sed -i -e "s#$orig#$dest#g" /etc/init.d/kamailio
           # Patch Init scripts = CFGFILE
             orig="KAMCFG=/etc/kamailio/kamailio.cfg"
             dest="KAMCFG=$REAL_PATH/etc/kamailio/kamailio.cfg"
             sed -i -e "s#$orig#$dest#g" /etc/init.d/kamailio

            # Enable startup
             chkconfig kamailio on

        else
           echo "Sorry, $DIST init scripts not *yet* supported! (feel free to contribute it!)"
        fi
   	   ;;
     n|N|* ) 
   	   echo "skipping..."
   	   ;;
   esac


### START APACHE2 ####

# set FQDN name variable
localweb=$(ip addr show dev $MAINIF  | grep inet -w | awk '{ print $2 }' | sed s#/.*##g )
echo -n "Enter webserver IP/FQDN (leave blank for '$localweb'): "
read dname
if [ "$dname" = "" ] ; then
        echo "Using default Vhostname '$localweb'..."
        dname=$localweb
fi
echo "Setting up Vhost name for $dname"
  
case $DIST in
        'DEBIAN')
        WEBROOT="/var/www"
        WEBSERV="apache2"
        # activate mod_rewrite
        a2enmod rewrite
        # Generate Certificates if not present
        if [ ! -d "/etc/ssl/localcerts/apache.key" ]; then
          mkdir -p /etc/ssl/localcerts
          openssl req -new -x509 -days 365 -nodes -out /etc/ssl/localcerts/apache.pem -keyout /etc/ssl/localcerts/apache.key
          chmod 600 /etc/ssl/localcerts/apache*
        fi
        # activate ssl
        a2enmod ssl

  echo "<VirtualHost *:80>
        ServerAdmin admin@$dname
        ServerName  $dname

        Redirect permanent / https://$dname/
</VirtualHost>

<VirtualHost *:443>
        ServerAdmin admin@$dname
        ServerName  $dname

        # Indexes + Directory Root.
        DirectoryIndex index.php index.html index.htm
        DocumentRoot $WEBROOT/webhomer

        <Directory />
                Options FollowSymLinks
                AllowOverride None
        </Directory>
        <Directory $WEBROOT/webhomer>
                Options Indexes FollowSymLinks MultiViews
                AllowOverride all
                Order allow,deny
                allow from all
        </Directory>

         SSLEngine on
         SSLCertificateFile /etc/ssl/localcerts/apache.pem
         SSLCertificateKeyFile /etc/ssl/localcerts/apache.key

</VirtualHost>" > /etc/apache2/sites-available/000-default.conf

        # activate
        service apache2 restart
        
  ;;
  'CENTOS')
        WEBROOT="/var/www/html"
        WEBSERV="httpd"
        # Generate Certificates if not present
        if [ ! -d "/etc/ssl/localcerts/apache.key" ]; then
          mkdir -p /etc/ssl/localcerts
          openssl req -new -x509 -days 365 -nodes -out /etc/ssl/localcerts/apache.pem -keyout /etc/ssl/localcerts/apache.key
          chmod 600 /etc/ssl/localcerts/apache*
        fi
        
        ## mod_rewrite already enable in centos by default
  echo "<VirtualHost *:80>
        ServerAdmin admin@$dname
        ServerName  $dname

        Redirect permanent / https://$dname/
</VirtualHost>

<VirtualHost *:443>
        ServerAdmin admin@$dname
        ServerName  $dname
        
        SSLEngine on
        SSLCertificateFile /etc/ssl/localcerts/apache.pem
        SSLCertificateKeyFile /etc/ssl/localcerts/apache.key

        # Indexes + Directory Root.
        DirectoryIndex index.php index.html index.htm
        DocumentRoot $WEBROOT/webhomer

        <Directory />
                Options FollowSymLinks
                AllowOverride None
        </Directory>
        <Directory $WEBROOT/webhomer>
                Options Indexes FollowSymLinks MultiViews
                AllowOverride all
                Order allow,deny
                allow from all
        </Directory>

</VirtualHost>" > /etc/httpd/conf.d/000-homer.conf
        sudo iptables -D INPUT -p tcp --dport 80 -j ACCEPT
        sudo iptables -D INPUT -p tcp --dport 443 -j ACCEPT
        sudo iptables -I INPUT -p tcp --dport 80 -j ACCEPT
        sudo iptables -I INPUT -p tcp --dport 443 -j ACCEPT
        sudo iptables-save
        sudo /sbin/service iptables save 
        ;;
esac


### END APACHE2 ######


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

   # Set Cron: Statistic
   read -p "Install Rotation Cronjob? (Y/n): " choice
   case "$choice" in
     y|Y|* )
           echo "Adding cronjob..."
   	# Set Cron: Partition Rotation 
   	rotate="/opt/partrotate_unixtimestamp.pl 2>&1> /dev/null"
   	job2="* 0 * * * sudo $rotate"
   	crontab -l > /opt/cron.tmp
   	echo "$job2" >> /opt/cron.tmp
   	CRON=$(cat /opt/cron.tmp | crontab - )
   	rm -rf /opt/cron.tmp
   	;;
     n|N ) echo "skipping";;
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

loadmodule "pv.so"
loadmodule "tm.so"
loadmodule "db_mysql.so"
loadmodule "sipcapture.so"
loadmodule "textops.so"
loadmodule "rtimer.so"
loadmodule "xlog.so"
loadmodule "sqlops.so"
loadmodule "htable.so"
loadmodule "sl.so"
loadmodule "siputils.so"

modparam("htable", "htable", "a=>size=8;autoexpire=400")
modparam("htable", "htable", "b=>size=8;autoexpire=31")

modparam("rtimer", "timer", "name=ta;interval=60;mode=1;")
modparam("rtimer", "exec", "timer=ta;route=TIMER_STATS")

modparam("sipcapture", "insert_retries", 5)
modparam("sipcapture", "insert_retry_timeout", 10)

' >> $config
echo "modparam(\"sqlops\", \"sqlcon\", \"cb=>mysql://$sqlhomeruser:$sqlhomerpassword@localhost/homer_db\")" >> $config
echo '

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
#read -p "Please confirm the local network device: [$MAINIF]: " device
 if [ "$device" = "" ] ; then
   device="$MAINIF"
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
modparam("sipcapture", "raw_interface", "$MAINIF")
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
modparam("sipcapture", "raw_interface", "$MAINIF")
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

cat - >> $config  <<'EOF'

# Main SIP request routing logic
# - processing of any incoming SIP request starts with this route
route {



        if($sht(a=>method::all) == $null) $sht(a=>method::all) = 0;
        $sht(a=>method::all) = $sht(a=>method::all) + 1;

        if($sht(b=>$rm::$cs::$ci) != $null) {
                $var(a) = "sip_capture";
                sip_capture("$var(a)");
                drop;
        }

        $sht(b=>$rm::$cs::$ci) = 1;

        if (is_method("INVITE|REGISTER")) {

                if($ua =~ "(friendly-scanner|sipvicious)") {
                        sql_query("cb", "INSERT INTO alarm_data_mem (create_date, type, total, source_ip, description) VALUES(NOW(), 'scanner', 1, '$si', 'Friendly scanner alarm!') ON DUPLICATE KEY UPDATE total=total+1");
                }

                #IP Method
                sql_query("cb", "INSERT INTO stats_ip_mem ( method, source_ip, total) VALUES('$rm', '$si', 1) ON DUPLICATE KEY UPDATE total=total+1");

                if($au != $null)  $var(anumber) = $au;
                else $var(anumber) = $fU;

                #hostname in contact
                if($sel(contact.uri.host) =~ "^(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})$") {
                        if($sht(a=>alarm::dns) == $null) $sht(a=>alarm::dns) = 0;
                        $sht(a=>alarm::dns) = $sht(a=>alarm::dns) + 1;
                }

                if($sel(contact.uri.host) != $si) {
                        if($sht(a=>alarm::spoofing) == $null) $sht(a=>alarm::spoofing) = 0;
                        $sht(a=>alarm::spoofing) = $sht(a=>alarm::spoofing) + 1;
                }

                if($au =~ "(\=)|(\-\-)|(\')|(\#)|(\%27)|(\%24)") {
                        if($sht(a=>alarm::sqlinjection) == $null) $sht(a=>alarm::sqlinjection) = 0;
                        $sht(a=>alarm::sqlinjection) = $sht(a=>alarm::sqlijnection) + 1;
                }

                if($(hdr(Record-Route)[0]{nameaddr.uri}) != $si) {
                        if($sht(a=>alarm::spoofing) == $null) $sht(a=>alarm::spoofing) = 0;
                        $sht(a=>alarm::spoofing) = $sht(a=>alarm::spoofing) + 1;
                }


                if (is_method("INVITE")) {

                        if (has_totag()) {
                                if($sht(a=>method::reinvite) == $null) $sht(a=>method::reinvite) = 0;
                                $sht(a=>method::reinvite) = $sht(a=>method::reinvite) + 1;
                        }
                        else {
                                if($sht(a=>method::invite) == $null) $sht(a=>method::invite) = 0;
                                $sht(a=>method::invite) = $sht(a=>method::invite) + 1;

                                if($adu != $null) {
                                        if($sht(a=>method::invite::auth) == $null) $sht(a=>method::invite::auth) = 0;
                                        $sht(a=>method::invite::auth) = $sht(a=>method::invite::auth) + 1;
                                }

                                if($ua != $null) {
                                        sql_query("cb", "INSERT INTO stats_useragent_mem (useragent, method, total) VALUES('$ua', 'INVITE', 1) ON DUPLICATE KEY UPDATE total=total+1");
                                }

                        }
                }
                else {
                        if($sht(a=>method::register) == $null) $sht(a=>method::register) = 0;
                        $sht(a=>method::register) = $sht(a=>method::register) + 1;

                        if($adu != $null) {
                                if($sht(a=>method::register::auth) == $null) $sht(a=>method::register::auth) = 0;
                                $sht(a=>method::register::auth) = $sht(a=>method::register::auth) + 1;
                        }

                        if($ua != $null) {
                                sql_query("cb", "INSERT INTO stats_useragent_mem (useragent, method, total) VALUES('$ua', 'REGISTER', 1) ON DUPLICATE KEY UPDATE total=total+1");
                        }
                }

        }
        else if(is_method("BYE")) {
                if($sht(a=>method::bye) == $null) $sht(a=>method::bye) = 0;
                $sht(a=>method::bye) = $sht(a=>method::bye) + 1;
                if(is_present_hf("Reason") && $(hdr(Reason){param.value,cause}{s.int}) != "" ) {
                       $var(cause) = $(hdr(Reason){param.value,cause}{s.int});
                       if($var(cause) != 16 && $var(cause) !=17) {
                                if($sht(a=>stats::sdf) == $null) $sht(a=>stats::sdf) = 0;
                                $sht(a=>stats::sdf) = $sht(a=>stats::sdf) + 1;
                       }
                }

        }
        else if(is_method("CANCEL")) {
                if($sht(a=>method::cancel) == $null) $sht(a=>method::cancel) = 0;
                $sht(a=>method::cancel) = $sht(a=>method::cancel) + 1;

        }
        else if(is_method("OPTIONS")) {
                if($sht(a=>method::options) == $null) $sht(a=>method::options) = 0;
                $sht(a=>method::options) = $sht(a=>method::options) + 1;

        }
        else if(is_method("REFER")) {
                if($sht(a=>method::refer) == $null) $sht(a=>method::refer) = 0;
                $sht(a=>method::refer) = $sht(a=>method::refer) + 1;

        }
        else if(is_method("UPDATE")) {
                if($sht(a=>method::update) == $null) $sht(a=>method::update) = 0;
                $sht(a=>method::update) = $sht(a=>method::update) + 1;
        }


        $var(a) = "sip_capture";
        # Kamailio 4.1 only
        #sip_capture("$var(a)"); 
        
        sip_capture();

        drop;
}

onreply_route {

        if($sht(a=>method::all) == $null) $sht(a=>method::all) = 0;
        $sht(a=>method::all) = $sht(a=>method::all) + 1;

        if($sht(b=>$rs::$cs::$rm::$ci) != $null) {
                $var(a) = "sip_capture";
                sip_capture("$var(a)");
                drop;
        }

        $sht(b=>$rs::$cs::$rm::$ci) = 1;

        #413 Too large
        if(status == "413") {

                if($sht(a=>alarm::413) == $null) $sht(a=>alarm::413) = 0;
                $sht(a=>alarm::413) = $sht(a=>alarm::413) + 1;
        }
        # Too many hops
        else if(status == "483") {
                if($sht(a=>alarm::483) == $null) $sht(a=>alarm::483) = 0;
                $sht(a=>alarm::483) = $sht(a=>alarm::483) + 1;

        }
        # loops
        else if(status == "482") {
                if($sht(a=>alarm::482) == $null) $sht(a=>alarm::482) = 0;
                $sht(a=>alarm::482) = $sht(a=>alarm::482) + 1;

        }
        # 400
        else if(status == "400") {
                if($sht(a=>alarm::400) == $null) $sht(a=>alarm::400) = 0;
                $sht(a=>alarm::400) = $sht(a=>alarm::400) + 1;

        }

        # 500
        else if(status == "500") {
                if($sht(a=>alarm::500) == $null) $sht(a=>alarm::500) = 0;
                $sht(a=>alarm::500) = $sht(a=>alarm::500) + 1;
        }
        # 503
        else if(status == "503") {
                if($sht(a=>alarm::503) == $null) $sht(a=>alarm::503) = 0;
                $sht(a=>alarm::503) = $sht(a=>alarm::503) + 1;
        }
        # 403
        else if(status == "403") {
                if($sht(a=>alarm::403) == $null) $sht(a=>alarm::403) = 0;
                $sht(a=>alarm::403) = $sht(a=>alarm::403) + 1;
        }
        # MOVED
        else if(status =~ "^(30[012])$") {
                if($sht(a=>response::300) == $null) $sht(a=>response::300) = 0;
                $sht(a=>response::300) = $sht(a=>response::300) + 1;
        }

        if($rm == "INVITE") {
                #ISA
                if(status =~ "^(408|50[03])$") {
                        if($sht(a=>stats::isa) == $null) $sht(a=>stats::isa) = 0;
                        $sht(a=>stats::isa) = $sht(a=>stats::isa) + 1;
                }
                #Bad486
                if(status =~ "^(486|487|603)$") {
                        if($sht(a=>stats::bad::invite) == $null) $sht(a=>stats::bad::invite) = 0;
                        $sht(a=>stats::bad::invite) = $sht(a=>stats::bad::invite) + 1;
                }

                #SD
                if(status =~ "^(50[034])$") {
                        if($sht(a=>stats::sd) == $null) $sht(a=>stats::sd) = 0;
                        $sht(a=>stats::sd) = $sht(a=>stats::sd) + 1;
                }

                if(status == "407") {
                        if($sht(a=>response::407::invite) == $null) $sht(a=>response::407::invite)= 0;
                        $sht(a=>response::407::invite) = $sht(a=>response::407::invite) + 1;
                }
                else if(status == "401") {
                        if($sht(a=>response::401::invite) == $null) $sht(a=>response::401::invite)= 0;
                        $sht(a=>response::401::invite) = $sht(a=>response::401::invite) + 1;
                }
                else if(status == "200") {
                        if($sht(a=>response::200::invite) == $null) $sht(a=>response::200::invite)= 0;
                        $sht(a=>response::200::invite) = $sht(a=>response::200::invite) + 1;
                }
        }
        else if($rm == "BYE") {

                if(status == "407") {
                        if($sht(a=>response::407::bye) == $null) $sht(a=>response::407::bye) = 0;
                        $sht(a=>response::407::bye) = $sht(a=>response::407::bye) + 1;
                }
                else if(status == "401") {
                        if($sht(a=>response::401::bye) == $null) $sht(a=>response::401::bye) = 0;
                        $sht(a=>response::401::bye) = $sht(a=>response::401::bye) + 1;
                }
                else if(status == "200") {
                        if($sht(a=>response::200::bye) == $null) $sht(a=>response::200::bye) = 0;
                        $sht(a=>response::200::bye) = $sht(a=>response::200::bye) + 1;
                }
        }

        sip_capture();

        drop;
}


route[TIMER_STATS] {

    xlog("timer routine: time is $TS\n");

    #ALARM SCANNERS
    sql_query("cb", "INSERT INTO alarm_data (create_date, type, total, source_ip, description) SELECT create_date, type, total, source_ip, description FROM alarm_data_mem;");
    sql_query("cb", "TRUNCATE TABLE alarm_data_mem");

    #413
    if($sht(a=>alarm::413) > 0) {
        sql_query("cb", "INSERT INTO alarm_data (create_date, type, total, description) VALUES(NOW(), 'Too Many 413', $sht(a=>alarm::413), 'Too many big messages')");
    }

    $sht(a=>alarm::413) = 0;

    #483
    if($sht(a=>alarm::483) > 0) {
        sql_query("cb", "INSERT INTO alarm_data (create_date, type, total, description) VALUES(NOW(), 'Too Many 483', $sht(a=>alarm::483), 'Too many hops messages')");
    }

    $sht(a=>alarm::483) = 0;

    #482
    if($sht(a=>alarm::482) > 0) {
        sql_query("cb", "INSERT INTO alarm_data (create_date, type, total, description) VALUES(NOW(), 'Too Many 482', $sht(a=>alarm::482), 'Too many loops messages')");
    }

    $sht(a=>alarm::482) = 0;

    #403
    if($sht(a=>alarm::403) > 0) {
        sql_query("cb", "INSERT INTO alarm_data (create_date, type, total, description) VALUES(NOW(), 'Too Many 403', $sht(a=>alarm::403), 'fraud alarm')");
    }
    $sht(a=>alarm::403) = 0;

    #503
    if($sht(a=>alarm::503) > 0) {
        sql_query("cb", "INSERT INTO alarm_data (create_date, type, total, description) VALUES(NOW(), 'Too Many 503', $sht(a=>alarm::503), 'service unavailable')");
    }
    $sht(a=>alarm::503) = 0;

    #500
    if($sht(a=>alarm::500) > 0) {
        sql_query("cb", "INSERT INTO alarm_data (create_date, type, total, description) VALUES(NOW(), 'Too Many 500', $sht(a=>alarm::500), 'server errors')");
    }
    $sht(a=>alarm::500) = 0;

    #408
    if($sht(a=>alarm::408) > 0) {
        sql_query("cb", "INSERT INTO alarm_data (create_date, type, total, description) VALUES(NOW(), 'Too Many 408', $sht(a=>alarm::408), 'Timeout')");
    }

    $sht(a=>alarm::408) = 0;

    #400
    if($sht(a=>alarm::400) > 0) {
        sql_query("cb", "INSERT INTO alarm_data (create_date, type, total, description) VALUES(NOW(), 'Too Many 400', $sht(a=>alarm::400), 'Too many bad request')");
    }
    $sht(a=>alarm::400) = 0;

    #delete old alarms
    sql_query("cb", "DELETE FROM alarm_data WHERE create_date < DATE_SUB(NOW(), INTERVAL 5 DAY)");

    #SQL STATS

    $var(tm) = ($time(min) mod 10);

    if($var(tm) != 0 && $var(tm) != 5) return;

    $var(t1) = $TS;
    $var(t2) = $var(t1) - 300;

    xlog("TIME : $var(tm)\n");

    $var(t_date) = "FROM_UNIXTIME(" + $var(t1) + ", '%Y-%m-%d %H:%i:00')";
    $var(f_date) = "FROM_UNIXTIME(" + $var(t2) + ", '%Y-%m-%d %H:%i:00')";

    #STATS Useragent
    sql_query("cb", "INSERT INTO stats_useragent (from_date, to_date, useragent, method, total) SELECT $var(f_date) as from_date, $var(t_date) as to_date, useragent, method, total FROM stats_useragent_mem;");
    sql_query("cb", "TRUNCATE TABLE stats_useragent_mem");

    #STATS IP
    sql_query("cb", "INSERT INTO stats_ip (from_date, to_date, method, source_ip, total) SELECT $var(f_date) as from_date, $var(t_date) as to_date, method, source_ip, total FROM stats_ip_mem;");
    sql_query("cb", "TRUNCATE TABLE stats_ip_mem");

    #INSERT SQL STATS
    #SDF
    if($sht(a=>stats::sdf) != $null && $sht(a=>stats::sdf) > 0) {
        sql_query("cb", "INSERT INTO stats_data (from_date, to_date, type, total) VALUES($var(f_date), $var(t_date), 'sdf', $sht(a=>stats::sdf))");
        $sht(a=>stats::sdf) = 0;
    }

    #ISA
    if($sht(a=>stats::isa) != $null && $sht(a=>stats::isa) > 0) {
        sql_query("cb", "INSERT INTO stats_data (from_date, to_date, type, total) VALUES($var(f_date), $var(t_date), 'isa', $sht(a=>stats::isa))");
        $sht(a=>stats::isa) = 0;
    }

    #SD
    if($sht(a=>stats::sd) != $null && $sht(a=>stats::sd) > 0) {
        sql_query("cb", "INSERT INTO stats_data (from_date, to_date, type, total) VALUES($var(f_date), $var(t_date), 'isa', $sht(a=>stats::sd))");
        $sht(a=>stats::sd) = 0;
    }

    #SSR
    if($sht(a=>stats::ssr) != $null && $sht(a=>stats::ssr) > 0) {
        sql_query("cb", "INSERT INTO stats_data (from_date, to_date, type, total) VALUES($var(f_date), $var(t_date), 'ssr', $sht(a=>stats::ssr))");
        $sht(a=>stats::ssr) = 0;
    }

    #ASR
    $var(asr) = 0;
    #if($sht(a=>response::200::invite) > 0) {
    if($sht(a=>method::invite) > 0) {
        if($sht(a=>response::407::invite) == $null) $sht(a=>response::407::invite) = 0;
        if($sht(a=>response::200::invite) == $null) $sht(a=>response::200::invite) = 0;
        $var(d) = $sht(a=>method::invite) - $sht(a=>response::407::invite);
        if($var(d) > 0) {
                $var(asr) =  $sht(a=>response::200::invite) / $var(d) * 100;
                if($var(asr) > 100)  $var(asr) = 100;
        }
    }

    #Stats DATA
    sql_query("cb", "INSERT INTO stats_data (from_date, to_date, type, total) VALUES($var(f_date), $var(t_date), 'asr', $var(asr))");


    #NER
    $var(ner) = 0;
    #if($sht(a=>response::200::invite) > 0 || $sht(a=>stats::bad::invite) > 0) {
    if($sht(a=>method::invite) > 0) {

        if($sht(a=>response::200::invite) == $null) $sht(a=>response::200::invite) = 0;
        if($sht(a=>response::bad::invite) == $null) $sht(a=>response::bad::invite) = 0;
        if($sht(a=>response::407::invite) == $null) $sht(a=>response::407::invite) = 0;

        $var(d) = $sht(a=>method::invite) - $sht(a=>response::407::invite);

        if($var(d) > 0) {
                $var(ner) =  ($sht(a=>response::200::invite) + $sht(a=>stats::bad::invite)) / $var(d) * 100;
                if($var(ner) > 100)  $var(ner) = 100;
        }
    }

    sql_query("cb", "INSERT INTO stats_data (from_date, to_date, type, total) VALUES($var(f_date), $var(t_date), 'ner', $var(ner))");

    #INVITE
    if($sht(a=>method::reinvite) > 0) {
        sql_query("cb", "INSERT INTO stats_method (from_date, to_date, method, totag, total) VALUES($var(f_date), $var(t_date),'INVITE', 1, $sht(a=>method::reinvite))");
        $sht(a=>method::reinvite) = 0;
    }

    #INVITE
    if($sht(a=>method::invite) > 0) {
        sql_query("cb", "INSERT INTO stats_method (from_date, to_date, method, total) VALUES($var(f_date), $var(t_date), 'INVITE', $sht(a=>method::invite))");
        $sht(a=>method::invite) = 0;
    }

    #INVITE AUTH
    if($sht(a=>method::invite::auth) > 0) {
        sql_query("cb", "INSERT INTO stats_method (from_date, to_date, method, auth, total) VALUES($var(f_date), $var(t_date), 'INVITE', 1, $sht(a=>method::invite::auth))");
        $sht(a=>method::invite::auth) = 0;
    }

    #REGISTER
    if($sht(a=>method::register) > 0) {
        sql_query("cb", "INSERT INTO stats_method (from_date, to_date, method, total) VALUES($var(f_date), $var(t_date), 'REGISTER', $sht(a=>method::register))");
        $sht(a=>method::register) = 0;
    }

    #REGISTER AUTH
    if($sht(a=>method::register::auth) > 0) {
        sql_query("cb", "INSERT INTO stats_method (from_date, to_date, method, auth, total) VALUES($var(f_date), $var(t_date), 'REGISTER', 1, $sht(a=>method::register::auth))");
        $sht(a=>method::register::auth) = 0;
    }

    #BYE
    if($sht(a=>method::bye) > 0) {
        sql_query("cb", "INSERT INTO stats_method (from_date, to_date, method, total) VALUES($var(f_date), $var(t_date), 'BYE', $sht(a=>method::bye))");
        $sht(a=>method::bye) = 0;
    }

    #CANCEL
    if($sht(a=>method::cancel) > 0) {
        sql_query("cb", "INSERT INTO stats_method (from_date, to_date, method, total) VALUES($var(f_date), $var(t_date), 'CANCEL', $sht(a=>method::cancel))");
        $sht(a=>method::cancel) = 0;
    }

    #OPTIONS
    if($sht(a=>method::options) > 0) {
        sql_query("cb", "INSERT INTO stats_method (from_date, to_date, method, total) VALUES($var(f_date), $var(t_date), 'OPTIONS', $sht(a=>method::options))");
        $sht(a=>method::options) = 0;
    }

    #REFER
    if($sht(a=>method::refer) > 0) {
        sql_query("cb", "INSERT INTO stats_method (from_date, to_date, method, total) VALUES($var(f_date), $var(t_date), 'REFER', $sht(a=>method::refer))");
        $sht(a=>method::refer) = 0;
    }

    #UPDATE
    if($sht(a=>method::update) > 0) {
        sql_query("cb", "INSERT INTO stats_method (from_date, to_date, method, total) VALUES($var(f_date), $var(t_date), 'UPDATE', $sht(a=>method::update))");
        $sht(a=>method::update) = 0;
    }

    #RESPONSE

    #300
    if($sht(a=>response::300) > 0) {
        sql_query("cb", "INSERT INTO stats_method (from_date, to_date, method, total) VALUES($var(f_date), $var(t_date), '300', $sht(a=>response::300))");
        $sht(a=>response::300) = 0;
    }

    #407 INVITE
    if($sht(a=>response::407::invite) > 0) {
        sql_query("cb", "INSERT INTO stats_method (from_date, to_date, method, cseq, total) VALUES($var(f_date), $var(t_date), '407', 'INVITE', $sht(a=>response::407::invite))");
        $sht(a=>response::407::invite) = 0;
    }

    #401 INVITE
    if($sht(a=>response::401::invite) > 0) {
        sql_query("cb", "INSERT INTO stats_method (from_date, to_date, method, cseq, total) VALUES($var(f_date), $var(t_date), '401', 'INVITE', $sht(a=>response::401::invite))");
        $sht(a=>response::401::invite) = 0;
    }

    #200 INVITE
    if($sht(a=>response::200::invite) > 0) {
        sql_query("cb", "INSERT INTO stats_method (from_date, to_date, method, cseq, total) VALUES($var(f_date), $var(t_date), '200', 'INVITE', $sht(a=>response::200::invite))");
        $sht(a=>response::200::invite) = 0;
    }

    #407 BYE
    if($sht(a=>response::407::bye) > 0) {
        sql_query("cb", "INSERT INTO stats_method (from_date, to_date, method, cseq, total) VALUES($var(f_date), $var(t_date), '407', 'BYE', $sht(a=>response::407::bye))");
        $sht(a=>response::407::bye) = 0;
    }

    #401 BYE
    if($sht(a=>response::401::bye) > 0) {
        sql_query("cb", "INSERT INTO stats_method (from_date, to_date, method, cseq, total) VALUES($var(f_date), $var(t_date), '401', 'BYE', $sht(a=>response::401::bye))");
        $sht(a=>response::401::bye) = 0;
    }

    #200 BYE
    if($sht(a=>response::200::bye) > 0) {
        sql_query("cb", "INSERT INTO stats_method (from_date, to_date, method, cseq, total) VALUES($var(f_date), $var(t_date), '200', 'BYE', $sht(a=>response::200::bye))");
        $sht(a=>response::200::bye) = 0;
    }

    #ALL MESSAGES
    if($sht(a=>method::all) > 0) {
        sql_query("cb", "INSERT INTO stats_method (from_date, to_date, method, total) VALUES($var(f_date), $var(t_date), 'ALL', $sht(a=>method::all))");
        $sht(a=>method::all) = 0;
    }

}

EOF

# Open HEP port for traffic
        sudo iptables -I INPUT -p tcp --dport $capport -j ACCEPT
        sudo iptables -I INPUT -p udp --dport $capport -j ACCEPT
        sudo iptables-save
        sudo /sbin/service iptables save 
        
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
echo "*************************************************************"
echo "      ,;;;;,                                                 "
echo "     ;;;;;;;;.     Congratulations! HOMER has been installed!"
echo "   ;;;;;;;;;;;;                                              "
echo "  ;;;;  ;;  ;;;;   <--------------- INVITE ---------------   "
echo "  ;;;;  ;;  ;;;;    --------------- 200 OK --------------->  "
echo "  ;;;;  ..  ;;;;                                             " 
echo "  ;;;;      ;;;;   Your system should be now ready to rock!"
echo "  ;;;;  ;;  ;;;;   Please verify/complete the configuration  "
echo "  ,;;;  ;;  ;;;;   files generated by the installer below.   "
echo "   ;;;;;;;;;;;;                                              "
echo "    :;;;;;;;;;     THIS SCRIPT IS PROVIDED AS-IS, USE AT     "
echo "     ;;;;;;;;      YOUR *OWN* RISK, REVIEW LICENSE & DOCS    "
echo "                                                             "
echo "*************************************************************"
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
echo "         http://$HOSTNAME/webhomer or http://$HOSTNAME"
echo "         [default login: test@test.com/test123]"
echo
echo "**************************************************************"
echo
echo " IMPORTANT: Do not forget to send Homer node some traffic! ;) "
echo " For our capture agent, visit http://captagent.googlecode.com "
echo " For more help and informations visit: http://sipcapture.org "
echo
echo "**************************************************************"
echo " Installer Log saved to: $logfile "
echo 
exit 0