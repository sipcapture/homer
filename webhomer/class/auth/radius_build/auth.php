<?php
/*
 * HOMER Web Interface
 * Homer's radius auth
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
require("class/auth/radius_build/settings.php");

class HomerAuthentication extends Authentication {


  function login($username, $password) {
    $radius = radius_auth_open(); 
    if (! radius_add_server($radius,RADIUS_HOST, RADIUS_PORT, RADIUS_SECRET, RADIUS_TIMEOUT, RADIUS_MAXTRIES)) 
    { 
        die('Radius Error: ' . radius_strerror($radius)); 
    } 

    if (! radius_create_request($radius,RADIUS_ACCESS_REQUEST)) 
    { 
        die('Radius Error: ' . radius_strerror($radius)); 
    } 
    
    radius_put_attr($radius,RADIUS_USER_NAME,$username); 
    radius_put_attr($radius,RADIUS_USER_PASSWORD,$password); 
    radius_put_attr($radius,RADIUS_NAS_IDENTIFIER,RADIUS_IDENTIFIER);

    $response = radius_send_request($radius);
    
    if($response == RADIUS_ACCESS_ACCEPT) {
	  $_SESSION['loggedin'] = $username;
    $_SESSION['userlevel'] = RADIUS_USERLEVEL;  //User level set in settings.php

    
          return true;
    }
    else if($response == RADIUS_ACCESS_CHALLENGE) {
          //Challenge
          return false;
    }

    return false;
    
  }
  
}
    
?>
