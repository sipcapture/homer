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
                $r = @ldap_search( $ds, LDAP_BASEDN, 'uid=' . $username);
                if ($r) {
                      $result = @ldap_get_entries( $ds, $r);
                      if ($result[0]) {
                          if (@ldap_bind( $ds, $result[0]['dn'], $password) ) {
                              if($result[0] != NULL) {
                                    $_SESSION['loggedin'] = $username;
                                    $_SESSION['userlevel'] = "3";  //All ldap clients are users                                                                                                    
                                    return true;
                              }
                          }                
                      }
                }
        }
	return false;
  }
}

