 #!/bin/sh
#
# --------------------------------------------------------------------------------
# Captagent automated installation script for Debian/CentOs/OpenSUSE (BETA)
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

VERSION=0.1.0
HOSTNAME=$(hostname)

clear; 
echo "*************************************************************"
echo "                                                             "
echo "      ,;;;;,       HOMER CAPTAGENT4  (http://sipcapture.org) "
echo "    .;;;;;;;;.     CaptAgent Auto-Installer (beta $VERSION)"
echo "   ;;;;;;;;;;;;                                              "
echo "  ;;;;^    ^;;;;   <------------ (HEP INVITE) ------------   "
echo "  ;;;;  ;;  ;;;;    -------------(HEP 200 OK) ------------>  "
echo "  ;;;;  ;;;;;;;;                                             " 
echo "  ;;;;  ;;;;;;;;                                             "
echo "  ;;;;  ;;  ;;;;   Project: http://captagent.googlecode.com  "
echo "  ,;;;,    ,;;;;                                             "
echo "   ;;;;;;;;;;;;                                              "
echo "    :;;;;;;;;;     THIS SCRIPT IS PROVIDED AS-IS, USE AT     "
echo "     ^;;;;;;^      YOUR *OWN* RISK, REVIEW LICENSE & DOCS    "
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
    echo "available at https://code.google.com/p/captagent/wiki/HOWTO"
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
    echo "available at https://code.google.com/p/captagent/wiki/HOWTO"
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
     echo "This script will download, compile and install the requirements for captagent automatically."
     echo
     echo "Enter captagent install path, press enter for default or 'abort' [/usr/local/etc/captagent]: "
     read  destination
     [ "$destination" = "abort" ] && return
     [ "$destination" = "" ] && destination="/usr/local/etc/captagent"
else
     destination=$1
fi

echo "Captagent will be installed to '$destination'"
echo
# Set full path, check if local
SETUP_ENV=$destination
echo "$SETUP_ENV" |grep '^/' -q && REAL_PATH=$SETUP_ENV || REAL_PATH=$PWD/$SETUP_ENV

GITREPO="https://code.google.com/p/captagent/ captagent"
LOCALDIR=`pwd`

# Setup Kamailio-DEV w/ sipcapture module from GIT
echo
echo "**************************************************************"
echo " INSTALLING OS PACKAGES AND DEPENDENCIES FOR CAPTAGENT"
echo "**************************************************************"
echo
echo "This might take a while depending on system/network speed. Please stand by...."
echo

case $DIST in
    'DEBIAN')
	     apt-get -y install git make m4 automake autoconf libtool libcap-dev libexpat-dev libpcap-dev zlib1g-dev
	     cd $LOCALDIR
  	     git clone $GITREPO
	     cd captagent/captagent
	     echo "Do you want to enable support for SSL transport or Payload-Compression? [Y/n]: "
	     read  enableSSL
	     case $enableSSL in
		  Y|y)
		     echo "SSL transport or Payload-Compression support enabled"
		     echo 
		     apt-get -y install openssl libssl-dev
		     echo
		     echo "Building..."
		     ./build.sh
		     echo
		     echo "Running Configure..."
		     echo
		     ./configure --enable-ssl --enable-compression
		  ;;
		  N|n)
		    echo "No SSL transport enabled"
		    echo
                    echo "Building..."
		    ./build.sh
		    echo
                    echo "Running Configure..."
                    echo
		    ./configure
		  ;;
		  *)
		    echo "SSL transport or Payload-Compression support enabled"
		    echo
		    apt-get -y install openssl libssl-dev
		    echo "Building..."
		    ./build.sh
		    echo 
                    echo "Running Configure..."
                    echo
                    ./configure --enable-ssl --enable-compression
    		    ;;
	      esac
	      make && make install
    ;;
    'CENTOS')
	     yum install -y git make m4 automake autoconf libtool libcap libcap-devel expat-devel libpcap-devel
	     cd $LOCALDIR
	     git clone $GITREPO
	     cd captagent/captagent
	     echo "Do you want to enable support for SSL transport or Payload-Compression? [Y/n]: "
	     read  enableSSL
	     case $enableSSL in
		  Y|y)
		     echo "SSL transport or Payload-Compression support enabled"
		     echo 
		     yum -y install openssl-devel
		     echo
		     echo "Building..."
		     ./build.sh
		     echo
		     echo "Running Configure..."
		     echo
		     ./configure --enable-ssl --enable-compression
		  ;;
		  N|n)
		    echo "No SSL transport enabled"
		    echo
                    echo "Building..."
		    ./build.sh
		    echo
                    echo "Running Configure..."
                    echo
		    ./configure
		  ;;
		  *)
		    echo "SSL transport or Payload-Compression support enabled"
		    echo
		    yum -y install libssl-dev openssl-devel
		    echo "Building..."
		    ./build.sh
		    echo 
                    echo "Running Configure..."
                    echo
                    ./configure --enable-ssl --enable-compression
		  ;;
	     esac

	     make && make install
    ;;
esac	

# Setup captagent init scripts
   echo
   read -p "Would you like to install Captagent's init/defaults scripts? (y/N): " choice
   case "$choice" in 
     y|Y ) 
        if [ "$DIST" == "DEBIAN" ]; then
           # INIT SCRIPTS
             cp $LOCALDIR/captagent/captagent/init/debian/captagent.init /etc/init.d/captagent
             chmod 755 /etc/init.d/captagent 
             
        elif  [ "$DIST" == "CENTOS" ]; then
           # INIT SCRIPTS
             cp $LOCALDIR/captagent/captagent/init/centos/captagent.init /etc/init.d/captagent
             cp $LOCALDIR/captagent/captagent/init/centos/captagent.sysconfig /etc/sysconfig/captagent
             chmod 755 /etc/init.d/captagent
             chmod 755 /etc/sysconfig/captagent
        else
           echo "Sorry, $DIST init scripts not *yet* supported! (feel free to contribute it!)"
        fi
           ;;
     n|N|* ) 
           echo "skipping..."
           ;;
   esac


echo
echo "###################################################"
echo "Installation complete, please check captagent help:"
echo "###################################################"
echo
captagent -h
echo
echo "------------------------------------------------------------------"
echo "Captagent4 cannot properly function without proper configuration."
echo "A default XML example has been installed on your system :"
echo
echo "     - $destination/captagent.xml "
echo
echo "For instructions: https://code.google.com/p/captagent/wiki/HOWTO"
echo
echo "-------------------------------------------------------------------"



# Configure Captagent4
   echo
   read -p "Would you like to configure captagent? (y/N): " choice
   case "$choice" in 
     y|Y ) 
   	   localip=$(ip addr show dev $device  | grep inet -w | awk '{ print $2 }' | sed s#/.*##g )
	   if [ -z "$localip" ] || [ $localip == "" ]; then 
	   	localip="127.0.0.1" 
	   fi

	   echo "Choose HOMER Server IP (blank for '$localip') : "
	   read homerip
	   if [ -z "$homerip" ] || [ $homerip == "" ]; then 
	   	homerip=$localip 
	   fi
	   echo "Choose HOMER Server PORT (blank for '5060') : "
	   read homerport
	   if [ -z "$homerport" ] || [ $homerport == "" ]; then 
	   	homerport="5060" 
	   fi

	   echo "Patching default configuration in: $destination/captagent.xml"
	   cp $destination/captagent.xml $destination/captagent.xml.bk

	   orig="<param name=\"capture-host\" value=\"capture.homercloud.org\"/>"
	   dest="<param name=\"capture-host\" value=\"$homerip\"/>"
	   sed -i -e "s#$orig#$dest#g" $destination/captagent.xml

	   orig="<param name=\"capture-port\" value=\"9000\"/>"
	   dest="<param name=\"capture-port\" value=\"$homerport\"/>"
	   sed -i -e "s#$orig#$dest#g" $destination/captagent.xml

           echo

	   read -p "Enable daemon mode? (y/N): " choice
	      case "$choice" in 
	        y|Y ) 
			orig="<param name=\"daemon\" value=\"false\"/>"
	   		dest="<param name=\"daemon\" value=\"true\"/>"
	   		sed -i -e "s#$orig#$dest#g" $destination/captagent.xml
	        ;;
	        n|N|* ) 
 		;;
	      esac
           echo "Configuration complete! Please review before running!"
	   echo "Location: $destination/captagent.xml"
           echo
   	;;
     n|N|* ) 
   	echo "NOTE: Before starting, adjust your configuration in: $destination/captagent.xml"
   	echo
   	;;
   esac