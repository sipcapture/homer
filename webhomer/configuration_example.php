<?php

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

/* CFLOW Options */
define(CFLOW_CLEANUP, 1); /* Automatic Cleanup of old Cflow images */
define(CFLOW_TIMEZONE, "Europe/Amsterdam"); /* Timezone that will display in Call Flow Ladder Diagram */
define(CFLOW_DIRECTION, 0); /* Callflow Direction */

/* Modules Options */
define(MODULES, 0);  /* Set to 1 Enable Statistic Modules */
define(STAT_OFFSET, "");  /* Force statistic time offset, ie: "+2 hours" */
define(STAT_RANGE, 24);  /* Statistic default span/range, in hours */

/* Search Results Options */
define(RESULTS_ORDER, "asc"); 
define(AUTOCOMPLETE, 0);  /* Enables autocomplete in FROM & TO fiels- WARNING: db intensive */

/* BLEG DETECTION */
define(BLEGDETECT, 1); /* always detect BLEG leg in CFLOW/PCAP*/
define(BLEGCID, "-0"); /* Lorenzo's style */

/* Database: mysql */
define(DATABASE,"mysql");

/* AUTH: internal, radius_build, ldap  */
define(AUTHENTICATION,"internal");

/* ALARM MAIL */
define(ALARM_FROMEMAIL,"homer@example.com");
define(ALARM_TOEMAIL,"admin@example.com");


?>
