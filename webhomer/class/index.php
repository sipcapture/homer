<?php
/*
 * HOMER Web Interface
 * Homer's cflow.php
 *
 * Copyright (C) 2011-2012 Alexandr Dubovikov <alexandr.dubovikov@gmail.com>
 * Copyright (C) 2011-2012 Lorenzo Mangani <lorenzo.mangani@gmail.com>
 *
 * The Initial Developers of the Original Code are
 *
 * Alexandr Dubovikov <alexandr.dubovikov@gmail.com>
 * Lorenzo Mangani <lorenzo.mangani@gmail.com>
 *
 * This program is free software; you can redistribute it and/or
 * modify it under the terms of the GNU General Public License
 * as published by the Free Software Foundation; either version 2
 * of the License, or (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program; if not, write to the Free Software
 * Foundation, Inc., 51 Franklin Street, Fifth Floor, Boston, MA  02110-1301, USA.
 *
*/

defined( '_HOMEREXEC' ) or die( 'Restricted access' );

session_start();

define('__ROOT__', dirname(dirname(__FILE__))); 

/* global configuration file */
if(!file_exists(__ROOT__."/configuration.php")) { die("Configuration not found. Please refer to the README file."); }
else require_once(__ROOT__."/configuration.php");

/* We can added postgresql support too */
require_once("class/database/".DATABASE.".php");
$db = new HomerDB;

/* AUTH CLASS */
require_once("class/auth/".AUTHENTICATION."/auth.php");
$auth = new HomerAuthentication;
$auth->encrypt = true; //set encryption
//SET DB to AUTH CLASS
$auth->setDB($db);

/* ALARM class */
require_once("class/alarms/index.php");
$alarm = new HomerAlarm;
//SET DB to Alarm CLASS
$alarm->setDB($db);
//$alarm->sendMail("test", "test2");

if(!defined("SKIPAUTH") && $auth->logincheck() == false){
        //do something if NOT logged in. For example, redirect to login page or display message.
        header("Location: index.php\r\n");
        exit;
}

/* Some extra functions  */
function getVar($name, $default, $request, $type) {
        $val = $_REQUEST[$name];
        if(!$val) $val = $default;
        $type = strtoupper($type);
        if(strcmp($type,"int") == 0) intval($val);
        else if(strcmp($type,"string") == 0) return strval($val);
        else return $val;
}

function detectIE()
{
    if (isset($_SERVER['HTTP_USER_AGENT']) && preg_match("/(MSIE|Internet Explorer)/", $_SERVER['HTTP_USER_AGENT']))
        return true;
    else
        return false;
}

?>