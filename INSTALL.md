[![Logo](http://sipcapture.org/data/images/sipcapture_header.png)](http://sipcapture.org)

# HOMER 5 

## Installation

Homer 5 is composed of separate elements:
 * [homer-api](https://github.com/sipcapture/homer-api): API Server and Backend Component
 * [homer-ui](https://github.com/sipcapture/homer-ui):  User-Interface and Frontend Component

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


#### HOMER:

* Clone repository of [Homer-API](https://github.com/sipcapture/homer-api)
	 
	 ```# git clone https://github.com/sipcapture/homer-api.git```
		
* Clone repository of [Homer-UI](https://github.com/sipcapture/homer-ui)
	 
	 ```# git clone https://github.com/sipcapture/homer-ui.git```

#### MYSQL:
* Create MySQL databases:
	* create database homer_data _( < homer-api/sql/schema_capture.sql)_
	* create database homer_configuration _( < homer-api/sql/schema_configuration.sql)_
	* create database homer_statistics _( < homer-api/sql/schema_statistics.sql)_
	* create sipcapture user with access rights on new databases

* Configure HOMER-API:
	* Move example file _homer-api/api/preferences_example.php_ to _preferences.php_
	* Move example file _homer-api/api/configuration_example.php_ to _configuration.php_
	* Edit _configuration.php_ with the required Database access details

* Configure & Install rotation script:
	* Copy the scripts/ directory on your system _(ie: /opt/sipcapture/)_
	* Add rotation script to cron once a day _(scripts/rotate.sh)_
	
#### KAMAILIO:
* Clone and Install Kamailio

		# git clone --depth 1 git://git.sip-router.org/sip-router kamailio
		# cd kamailio; make PREFIX=$REAL_PATH FLAVOUR=kamailio include_modules="db_mysql sipcapture pv textops rtimer xlog sqlops htable sl siputils" cfg
		# make all && make install

		
* Move and Customize the provided sipcapture kamailio.cfg

		# cp homer-api/examples/sipcapture/kamailio.cfg /usr/local/etc/kamailio/kamailio.cfg
		
* Start Kamailio
	* Start a remote Capture Agent to send HEP packets to the selected HEP socket

#### HTTP Server:
* Configure Apache2 or nginx with a Homer vhost _(see examples/web/)_
* Install Homer 5 web components
	* Copy HOMER-UI Contents to vhost directory
	* Copy HOMER-API/api directory to vhost directory

-----------------

### Login to Homer-UI
	* Default Credentials: admin / test123 (see the schema_configuration.sql)

### Configure Homer-UI

	* Import the Example Admin Dashboard (_1430318378410.json) and customize Nodes, Users, Aliases
	* Import the SIP Search Dashboard (_1431943495.json) and perform a test search
	
