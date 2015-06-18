![Logo](http://sipcapture.org/data/images/sipcapture_header.png)

# HOMER 5 
This documents outlines the installation process for SIPCAPTURE HOMER version 5.x

![H5](http://i.imgur.com/G5LF1Wl.png)

## Installation

Homer 5 is composed of separate elements:
 * [HOMER-API](https://github.com/sipcapture/homer-api): API Server and Backend Component
 * [HOMER-UI](https://github.com/sipcapture/homer-ui):  User-Interface and Frontend Component

## Setup Guide

* Sytem Requirements:
	- Apache2 or nginx 
	- PHP-5, MySQL + InnoDB (barracuda) _( >= 5.6)_
	- Kamailio + sipcapture 
	- Homer-API + Homer-UI

* Remote Requirements:
	- HEP3 Capture Agent
		- [CaptAgent](https://github.com/sipcapture/captagent)
		- [Kamailio](https://github.com/kamailio/kamailio), [OpenSIPS](http://opensips.org/), [FreeSwitch](http://freeswitch.org/), [Asterisk](http://www.asterisk.org/)

## Locations

* $WEB: Homer web folder
* $GIT: Git source clone folder

#### HOMER:

* Clone repository of [Homer-API](https://github.com/sipcapture/homer-api)
	 
	 ```# git clone https://github.com/sipcapture/homer-api.git```
		
* Clone repository of [Homer-UI](https://github.com/sipcapture/homer-ui)
	 
	 ```# git clone https://github.com/sipcapture/homer-ui.git```

#### HTTP Server:
* Create a folder for HOMER vhost _($WEB)_
* Configure Apache2 or nginx for HOMER vhost _(see $GIT/homer-api/examples/web/)_
* Install Homer 5 web components
	* Copy HOMER-UI Contents to vhost directory
	* Copy HOMER-API/api directory to vhost directory


#### MYSQL:
* Create MySQL databases:
	* create database homer_data _( < $GIT/homer-api/sql/schema_capture.sql)_
	* create database homer_configuration _( < $GIT/homer-api/sql/schema_configuration.sql)_
	* create database homer_statistics _( < $GIT/homer-api/sql/schema_statistics.sql)_
	* create sipcapture user with access rights on new databases

* Configure HOMER-API:
	* Move example file _$GIT/homer-api/api/preferences_example.php_ to _$WEB/preferences.php_
	* Move example file _$GIT/homer-api/api/configuration_example.php_ to _$WEB/configuration.php_
	* Edit _$WEB/configuration.php_ with the required Database access details

* Configure & Install rotation script:
	* Copy the scripts/ directory on your system _(ie: /opt/sipcapture/)_
	* Add rotation script to cron once a day _(scripts/rotate.sh)_
	
#### KAMAILIO:
* Clone and Install Kamailio

		# git clone --depth 1 git://git.sip-router.org/sip-router kamailio
		# cd kamailio; make PREFIX=$REAL_PATH FLAVOUR=kamailio include_modules="db_mysql sipcapture pv textops rtimer xlog sqlops htable sl siputils" cfg
		# make all && make install

		
* Copy and Customize the provided sipcapture _kamailio.cfg_

		# cp homer-api/examples/sipcapture/kamailio.cfg /usr/local/etc/kamailio/kamailio.cfg
		
* Start Kamailio
	* Start a remote Capture Agent to send HEP packets to the selected HEP socket


-----------------

### Login to Homer-UI

* Login to Homer-UI using default user
	* admin / test123 _(see $GIT/homer-api/sql/schema_configuration.sql)_

### Configure Homer-UI

* Import the Example _Admin Dashboard_ and customize Nodes, Users, Aliases
	* _homer-api/examples/dashboards/_1430318378410.json_
* Import the _SIP Search Dashboard_ and perform a test search
	* _homer-api/examples/dashboards/_1431943495.json_

### Enjoy Homer5

![H5_dash](http://i.imgur.com/CT1BBGD.png)
