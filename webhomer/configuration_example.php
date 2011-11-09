<?php

define(CONFIG_VERSION, "1.0.3"); /* Please ALWAYS include CONFIGVERSION */
define(WEBHOMER_VERSION, "3.0.1"); /* WEBHOMER VERSION */

/* Access db of homer */
define(HOST, "localhost");
define(USER, "root");
define(PW, "root");
define(DB, "homer_users");

/* Homer connection 
*  this user must have the same password for all Homer nodes
*  please define all your nodes in homer_nodes table
*/
define(HOMER_HOST, "localhost");
define(HOMER_USER, "homer_user");
define(HOMER_PW, "homer_password");
define(HOMER_DB, "homer_db");
define(HOMER_TABLE, "sip_capture");

/* webHomer Settings 
*  Adjust to reflect your system preferences
*/

define(PCAPDIR,"/var/www/webhomer/tmp/");
define(WEBPCAPLOC,"/webhomer/tmp/");
define(APILOC,"/webhomer/api/");
define(COLORTAG, 0); /* color based on callid, fromtag */
define(DAYNIGHT, 1); /* day/night based theme, 0=day, 1=rotate, 3=night */
define(CHARTER, 2); /* 1: Highcharts, 2: Flot */
define(AJAXTYPE,"POST"); /* AJAX request type can be POST or GET */


/* CFLOW Options */
define(CFLOW_CLEANUP, 1); /* Automatic Cleanup of old Cflow images */
define(CFLOW_TIMEZONE, "Europe/Amsterdam"); /* Timezone that will display in Call Flow Ladder Diagram */
define(CFLOW_DIRECTION, 0); /* Callflow Direction */
define(CFLOW_FACTOR, 1);   /* IMAGE size factor. The value can be float */


/* Modules Options */
define(MODULES, 0);  /* Set to 1 Enable Statistic Modules */
define(STAT_OFFSET, "");  /* Force statistic time offset, ie: "+2 hours" */
define(STAT_RANGE, 24);  /* Statistic default span/range, in hours */

/* Search Results Options */
define(RESULTS_ORDER, "asc"); 
define(AUTOCOMPLETE, 0);  /* Enables autocomplete in FROM & TO fiels- WARNING: db intensive */

/* BLEG DETECTION */
define(BLEGDETECT, 0); /* always detect BLEG leg in CFLOW/PCAP*/
define(BLEGCID, "x-cid"); /* default Homer style. If you want use it, please add INDEX for callid_aleg */

/* Database: mysql */
define(DATABASE,"mysql");

/* AUTH: internal, radius_build, ldap  */
define(AUTHENTICATION,"internal");

/* ALARM MAIL */
define(ALARM_FROMEMAIL,"homer@example.com");
define(ALARM_TOEMAIL,"admin@example.com");

/* configuration check */
define(NOCHECK, 0); /* set to 1, dont check config */

/* ACCESS LEVEL 3 - Users, 2 - Manager, 1 - Admin, 0 - nobody */
define(ACCESS_SEARCH, 3); /* SEARCH FOR ALL:*/
define(ACCESS_TOOLBOX, 1); /* TOLBOX FOR ADMIN */
define(ACCESS_STATS, 3); /* SEARCH FOR ALL */
define(ACCESS_ADMIN, 1); /* SEARCH FOR ALL */

?>
