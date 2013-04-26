<?php
/*
 * HOMER Web Interface
 * Homer's index.php
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

if (!defined('_HOMEREXEC')) define('_HOMEREXEC', 1);
define('SKIPAUTH', 1);

require_once ('login.html.php');

class Component {

       function executeComponent () {
           global $auth, $task;

           HTML_login::displayLoginForm();

           if($task == "do") {                      
             	   //do something on FAILED login
             	   // echo "<font color='red'>Authentication Failed!</font>";
		   echo "<script> $('#logintext').prepend('<font color=red>Username/Password failure!<br></font>'); </script> ";	
	   }
	   
	}
}
