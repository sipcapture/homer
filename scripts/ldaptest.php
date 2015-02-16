<?php
	define('LDAP_HOST',"10.0.0.1");
	define('LDAP_PORT',389);
	define('LDAP_BASEDN',"<base dn where all users are located>");
	define('LDAP_REALM',"My Realm");
	define('LDAP_USERNAME_ATTRIBUTE',"AccountName");
	define('LDAP_USERLEVEL',1); 
	define('LDAP_ADMINLEVEL',1);
	define('LDAP_ADMIN_USER',"test");
	define('LDAP_BIND_USER', "Homer");	
	define('LDAP_BIND_PASSWORD', "<password>");
	define('LDAP_ENCRYPTION', "tls");	

	$username = "yourname";
	$password = yourpassword;
	$done = false;

	$ds=@ldap_connect(LDAP_HOST,LDAP_PORT);
		
	// Set LDAP Version, Default is Velrsion 2
	@ldap_set_option($ds, LDAP_OPT_PROTOCOL_VERSION, ( LDAP_VERSION) ? LDAP_VERSION : 2);
		
	// Referrals are disabled
	@ldap_set_option($ds, LDAP_OPT_REFERRALS, 0 );
	        
	// Enable TLS Encryption
	if(LDAP_ENCRYPTION == "tls") {
                     // Documentation says - set to never
		     putenv('LDAPTLS_REQCERT=never') or die('Failed to setup the env');
                    @ldap_start_tls($ds);
        }
	
	if (defined('LDAP_BIND_USER') && defined('LDAP_ADMIN_USER')) {
              if (!@ldap_bind( $ds, LDAP_BIND_USER, LDAP_BIND_PASSWORD)) {
                    return false;
               }
        }
	
        $r=@ldap_search( $ds, LDAP_BASEDN, LDAP_USERNAME_ATTRIBUTE . '=' . $username);
        if ($r) {
                $result = @ldap_get_entries( $ds, $r);
		if ($result[0]) {
                          if (@ldap_bind( $ds, $result[0]['dn'], $password) ) {
                              if($result[0] != NULL) {
                                    if (defined(LDAP_GROUPDN)) {
                                        if (!$this->check_filegroup_membership($ds,$username)) {
                                            return false;
                                        }
                                    }
                                    $_SESSION['loggedin'] = $username;
                                    $_SESSION['userlevel'] = LDAP_USERLEVEL;
				   
				    // Assigne Admin Privs, should be read from the LDAP Directory in the future 
				    $ADMIN_USER = split(",", LDAP_ADMIN_USER);
				    foreach($ADMIN_USER as &$value) {
					if ($value == $username) {
					  $_SESSION['userlevel'] = 1; # LDAP_ADMINLEVEL;
					}
				    }
                        	    $done = true;
                              }
                          }                
                }
        }

	if($done) echo "Well done!\n";
	else echo "not auth!\n";

?>
