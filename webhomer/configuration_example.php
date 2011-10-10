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

/* Settings 
*/

define(PCAPDIR,"/var/www/html/webhomer/tmp/");
define(WEBPCAPLOC,"/webhomer/tmp/");
define(COLORTAG, 0); /* color based on callid, fromtag */
define(MODULES, 0); /* display modules in homepage flag */
define(CFLOW_CLEANUP, 1); /* Automatic Cleanup of old Cflow images */
define(CFLOW_TIMEZONE, "Europe/Berlin"); /* Timezone that will display in Call Flow Ladder Diagram */

?>
