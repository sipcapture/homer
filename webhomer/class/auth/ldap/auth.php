<?php
/*
 * HOMER Web Interface
 * Homer's auth.php
 *
 * Copyright (C) 2011-2012 Alexandr Dubovikov <alexandr.dubovikov@gmail.com>
 * Copyright (C) 2011-2012 Lorenzo Mangani <lorenzo.mangani@gmail.com>
 *
 * The Initial Developers of the Original Code are
 *
 * Alexandr Dubovikov <alexandr.dubovikov@gmail.com>
 * Lorenzo Mangani <lorenzo.mangani@gmail.com>
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
*/

defined( '_HOMEREXEC' ) or die( 'Restricted access' );

require("class/auth/index.php");
require("class/auth/ldap/settings.php");

class HomerAuthentication extends Authentication {

  function login($username, $password) {

	if ($username != "" && $password != "") {
  
	  
    $ds=@ldap_connect(LDAP_HOST,LDAP_PORT);
		
		// Set LDAP Version, Default is Version 2
		@ldap_set_option($ds, LDAP_OPT_PROTOCOL_VERSION, ( LDAP_VERSION) ? LDAP_VERSION : 2);
		
		// Referrals are disabled
		@ldap_set_option($ds, LDAP_OPT_REFERRALS, 0 );
	        
		// Enable TLS Encryption
		if(LDAP_ENCRYPTION == "tls") {
      
	        	  // Documentation says - set to never
		          putenv('LDAPTLS_REQCERT=never') or die('Failed to setup the env');

              @ldap_start_tls($ds);
	        }
		
                $r=@ldap_search( $ds, LDAP_BASEDN, 'uid=' . $username);
                if ($r) {
                     $result = @ldap_get_entries( $ds, $r);
                      
			if ($result[0]) {
                          if (@ldap_bind( $ds, $result[0]['dn'], $password) ) {
                              if($result[0] != NULL) {
                                    if (LDAP_GROUPDN != NULL) {
                                        if (!$this->check_filegroup_membership($ds,$username)) {
                                            return false;
                                        }
                                    }
				    // Default each user has normal User Privs
                                    $_SESSION['loggedin'] = $username;
                                    $_SESSION['userlevel'] = LDAP_USERLEVEL;
				   
				    // Assigne Admin Privs, should be read from the LDAP Directory in the future 
				    $ADMIN_USER = split(",", LDAP_ADMIN_USER);
				    foreach($ADMIN_USER as &$value) {
							
					if ($value == $username) {
					  $_SESSION['userlevel'] = 1; # LDAP_ADMINLEVEL;
					}
				    }
                                    return true;
                              }
                          }                
                      }
                }
        }
	return false;
  }

  /* posixGroup schema, rfc2307 */
  function check_filegroup_membership($ds, $uid) {
    $dn = LDAP_GROUPDN;
    $attr = "memberUid";

    $result = @ldap_compare($ds, $dn, $attr, $uid);

    if ($result === true) {
        return true;
    } else {
        return false;
    }
  }
}

