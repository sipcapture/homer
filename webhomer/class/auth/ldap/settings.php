<?php

define('LDAP_HOST',"localhost");
define('LDAP_PORT',NULL);
define('LDAP_BASEDN',"dc=example,dc=com");
define('LDAP_REALM',"My Realm");
define('LDAP_USERLEVEL',3); //All ldap clients are user
// Require membership of group to login
#define('LDAP_GROUPDN',"cn=groupname,dc=example,dc=com");

?>
