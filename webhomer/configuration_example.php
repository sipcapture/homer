<?php

/*********************************************************************************/
/* Access db of homer */
define('HOST', "localhost");
define('PORT', 3306);
define('USER', "root");
define('PW', "root");
define('DB', "homer_users");

/* Homer connection 
*  this user must have the same password for all Homer nodes
*  please define all your nodes in homer_nodes table
*/
define('HOMER_HOST', "localhost"); /* DEFAULT. Don't forget insert this host to your DB nodes table */
define('HOMER_PORT', 3306);
define('HOMER_USER', "homer_user");
define('HOMER_PW', "homer_password");
define('HOMER_DB', "homer_db");
define('HOMER_TABLE', "sip_capture");

/*********************************************************************************/

/* webHomer Settings 
*  Adjust to reflect your system preferences
*/

define('PCAPDIR',"/var/www/webhomer/tmp/");
define('WEBPCAPLOC',"/webhomer/tmp/");
define('APIURL',"http://localhost");
define('APILOC',"/webhomer/api/");

/* INCLUDE preferences */

include("preferences.php");

?>
