<?php

define('LDAP_HOST',"localhost");
define('LDAP_PORT',NULL);
define('LDAP_BASEDN',"dc=example,dc=com");
define('LDAP_REALM',"My Realm");
define('LDAP_USERLEVEL',3); //All ldap clients are user

// To define Admin-User while using LDAP Auth. 
// Should be added to LDAP Directory later on
define('LDAP_ADMINLEVEL',1); 
define('LDAP_ADMIN_USER',"user1,user2,user3,user4");

// If not defined, Password my be commited unencrypted to ldap server
#define('LDAP_ENCRYPTION', "tls");
// LdapVersion. Default is 2
# define('LDAP_VERSION', "3");
// Require membership of group to login
#define('LDAP_GROUPDN',"cn=groupname,dc=example,dc=com");

?>

