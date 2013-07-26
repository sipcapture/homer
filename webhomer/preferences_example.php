<?php

define('HOMER_TIMEZONE', "America/Detroit"); /* Set a global application default timezone */

define('CONFIG_VERSION', "1.0.10"); /* Please ALWAYS include CONFIGVERSION */
define('WEBHOMER_VERSION', "3.5.0"); /* WEBHOMER VERSION */

/* Search params */
define('COLORTAG', 0); /* color based on callid, fromtag */
define('DAYNIGHT', 1); /* day/night based theme, 0=day, 1=rotate, 2=night */
define('CHARTER', 2); /* 1: Highcharts, 2: Flot (suggested) */
define('AJAXTYPE',"POST"); /* AJAX request type can be POST or GET */
define('AJAXTIMEOUT',60000); /* AJAX timeout request. Milliseconds! */

/* CFLOW Options */
define('CFLOW_CLEANUP', 1); /* Automatic Cleanup of old Cflow images */
define('CFLOW_TIMEZONE', HOMER_TIMEZONE); /* Timezone that will display in Call Flow Ladder Diagram */
define('CFLOW_DIRECTION', 0); /* Callflow Direction */
define('CFLOW_FACTOR', 1);   /* IMAGE size factor. The value can be float */
define('CFLOW_POPUP',1); /* Modal type: 1, Browser popup: 2 */
define('CFLOW_HPORT', 0); /* Column/Host Mode = Plain: 0, +Port: 1, Auto-Select: 2 */
define('CFLOW_EPORT', 0); /* Enable Ephemeral port detection, experimental */
define('MESSAGE_POPUP',1); /* Modal type: 1, Browser popup: 2 */

/* Modules Options */
define('MODULES', 0);  /* Set to 1 Enable Statistic Modules */
define('STAT_OFFSET', "");  /* Force statistic time offset, ie: "+2 hours" */
define('STAT_RANGE', 24);  /* Statistic default span/range, in hours */
define('ADMIN_DBSTAT', 0);  /* Display db status in admin page, MySQL only */
define('ADMIN_NETSTAT', 0);  /* Display network packets in admin page */

/* Search Results Options */
define('RESULTS_ORDER', "asc"); 
define('AUTOCOMPLETE', 0);  /* Enables autocomplete in FROM & TO fiels- WARNING: db intensive */
define('FORMAT_DATE_RESULT', "H:i:s"); /* Controls the Date/Time output in search results, ie: "m-d H:i:s"  */

/* BLEG DETECTION */
define('BLEGDETECT', 0); /* always detect BLEG leg in CFLOW/PCAP*/
define('BLEGCID', "x-cid"); /* options: x-cid, b2b */
define('BLEGTAIL', "-0"); /* session-ID correlation suffix, required for b2b mode */

/* Database: mysql */
define('DATABASE',"mysql");

/* AUTH: internal, radius_build, ldap  */
define('AUTHENTICATION',"internal");
// define('AUTHENTICATION_TEXT',"Please login with your credentials");

/* ALARM MAIL */
define('ALARM_FROMEMAIL',"homer@example.com");
define('ALARM_TOEMAIL',"admin@example.com");

/* configuration check */
define('NOCHECK', 0); /* set to 1, dont check config */

/* ACCESS LEVEL 3 - Users, 2 - Manager, 1 - Admin, 0 - nobody */
define('ACCESS_DASHBOARD', 3); /* ALARM FOR ALL:*/
define('ACCESS_ALARM', 3); /* ALARM FOR ALL:*/
define('ACCESS_SEARCH', 3); /* SEARCH FOR ALL:*/
define('ACCESS_TOOLBOX', 1); /* TOLBOX FOR ADMIN */
define('ACCESS_STATS', 3); /* STATS FOR ALL */
define('ACCESS_ADMIN', 1); /* ADMIN FOR ADMIN */
define('ACCESS_ACCOUNT', 3); /* ACCOUNT FOR ALL:*/

/* WEBKIT ENABLE */
define('WEBKIT_SPEECH', 1); /* enable:1 , disable: 0 */

/* LOGGING. to enable set bigger as 0, if 10 == 10 days keep logs */
define('SEARCHLOG', 0);

/* IP GeoLocation Links (requires client internet access) */
define('GEOIP_LINK', 0); /* 0, Disabled - 1, External Query GEOIP_URL, 2, Internal PopUp */
define('GEOIP_URL', "http://www.infosniper.net/?ip_address=");

/* ADMIN SERVICE MONITORING */
define('SERVICE_MONITOR',0);
define('SERVICE_HTTP_PORT', 80);
define('SERVICE_SMTP_PORT', 25);
define('SERVICE_SSH_PORT', 22);
define('SERVICE_SIP_PORT', 5060);

/* PCAP Exporting: Set to '0' for Local Shark, '1' for Cloud Shark  */
/* WARNING: Internet routes or proxy defaults REQUIRED to use CloudShark */
define('CSHARK', 0);
define('CSHARK_API', "2468738734d4f9db0d4b65db0c5daa3d"); /* Homer generic key, request yours if needed */
define('CSHARK_URI', "http://www.cloudshark.org");

/* SKIP AUTH for CFLOW/PCAP */
define('SKIPCFLOWAUTH', 0);

/*DEFAULT SELECTED DB NODE */
define('DEFAULTDBNODE',1);
  
/* PCAP Import: Streams to your HEP capture socket - REQUIRES CAPTAGENT INSTALLED! */
/* Use for CAPTAGENT 4.x */
// define('PCAP_AGENT4', 'captagent'); /* REQUIRES existing captagent4 XML configuration! test manually using -D option */
/* Use for old CAPTAGENT 0.x */
// define('PCAP_AGENT', 'captagent');
// define('PCAP_HEP_IP', "192.168.1.100");
// define('PCAP_HEP_PORT', "5060");

define('SESSION_NAME',"HOMERSESSID"); /* session ID name. */

/* SQL SCHEMA VERSION */
define('SQL_SCHEMA_VERSION',1); /* SQL SCHEMA VERSION. Default 1 */


?>
