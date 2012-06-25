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

/* Changed to PDO */
require_once("class/database/pdo.php");
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

$task = getVar('task', NULL, '', 'string');
$component = getVar('component', 'search', '', 'string');
$userlevel =  $_SESSION['userlevel'];
$header =  getVar('component', 0, '', 'int');

/* My Nodes */
$mynodeshost = array();
$mynodesname = array();
$nodes = $db->getAliases('nodes');
foreach($nodes as $node) {
        $mynodeshost[$node->id] = $node->host;
        $mynodesname[$node->id] = $node->name;
}

/* SECURITY LEVEL: 1 - Admin, 2 - Manager, 3 - User, 4 - Guest*/
$components = array("search" => ACCESS_SEARCH, "toolbox" => ACCESS_TOOLBOX, "statistic" =>ACCESS_STATS, "admin" => ACCESS_ADMIN);

/* Disable stats changing security level */
if(detectIE()) {
  //$components["statistic"]=0;
  define('IERROR',1);
}
else define('IERROR',0);

#Extra Security check
$security = 0;
foreach($components as $key=>$value) {

        if($key == $component) {
                if($userlevel <= $value) $security = 1;
                break;
        }
}


/* AUTH */
if($component == "login" && $task == "do") {
	if($auth->login($_REQUEST['username'], $_REQUEST['password']) == true){
	        header("Location: index.php?component=search\n\n");
        	exit;
	}
}

//if((!defined("SKIPAUTH") || $component != "login") && $auth->logincheck() == false){
if($auth->logincheck() == false){
  if((!defined('SKIPCFLOWAUTH') || SKIPCFLOWAUTH == 0) && preg_match('/(cflow.php|pcap.php)$/', $_SERVER['SCRIPT_URL'])) die('Login at first');
	$component = "login";	
	$security = 1;
}

/* Some extra functions  */
function getVar($name, $default, $request, $type) {        
        // Thank Iank Blenke for this fix
        $val = isset($_REQUEST[$name]) ? $_REQUEST[$name] : $default;        
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
