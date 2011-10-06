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
*  Be shure, that you have the last version of text2pcap
*  text2pcap shall support -4 -6 IP headers
*
*  Text2pcap: https://bugs.wireshark.org/bugzilla/show_bug.cgi?id=5650
*  Callflow: http://sourceforge.net/projects/callflow/
*
*/

define(PCAPDIR,"/var/www/html/webhomer/tmp/");
define(WEBPCAPLOC,"/webhomer/tmp/");
define(TEXT2PCAP,"/usr/sbin/text2pcap");
define(MERGECAP,"/usr/sbin/mergecap");
define(COLORTAG, 0); /* color based on callid, fromtag */
define(MODULES, 0); /* display modules in homepage flag */
define(CFLOW_CLEANUP, 1); /* Automatic Cleanup of old Cflow images */
define(CFLOW_TIMEZONE, "Europe/Berlin"); /* Timezone that will display in Call Flow Ladder Diagram */

?>
