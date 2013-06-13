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

define('__ROOT__', dirname(dirname(__FILE__))); 

set_include_path(get_include_path() . PATH_SEPARATOR . __ROOT__);

/* global configuration file */
if(!file_exists(__ROOT__."/configuration.php")) { die("Configuration not found. Please refer to the README file."); }
else require_once(__ROOT__."/configuration.php");

  /* time zone */
date_default_timezone_set(HOMER_TIMEZONE);

/* if defined session_name, set this */
if(defined('SESSION_NAME')) session_name(SESSION_NAME);

session_start();

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

$task = getVar('task', NULL, $_REQUEST, 'string');
$component = getVar('component', 'search', $_REQUEST, 'string');
if( !empty($_SESSION['userlevel']) )
    $userlevel =  $_SESSION['userlevel'];
else
    $userlevel = 'default';
$header =  getVar('component', 0, $_REQUEST, 'int');


class Node{
        public $name;
        public $host;
        public $dbport;
        public $dbname;
        public $dbusername;
        public $dbpassword;
        public $dbtablescnt;
        public $dbtables;
};

$mynodes = array();

/* My Nodes */
$nodes = $db->getNodes();
foreach($nodes as $node) {

        $mynodes[$node->id] = new Node();
	$mynodes[$node->id]->name = $node->name;

        $mynodes[$node->id]->dbtablescnt = array();
        $mynodes[$node->id]->dbtables = array();

        $tables = $node->dbtables;
        $i = 0;
        $tok = strtok ($tables, ", \t");
        while ($tok !== false)
        {
                $mynodes[$node->id]->dbtables[$i] = $tok;
                $mynodes[$node->id]->dbtablescnt[$i] = 0;
                $tok = strtok(", \t");
                $i = $i + 1 ;

        }


        if(count($mynodes[$node->id]->dbtables) == 0) $mynodes[$node->id]->dbtables = array(HOMER_TABLE);
        if(count($mynodes[$node->id]->dbtablescnt) == 0) $mynodes[$node->id]->dbtablescnt = array(1);


        $mynodes[$node->id]->dbname = isset($node->dbname) ? $node->dbname : HOMER_DB;
        $mynodes[$node->id]->host = isset($node->host) ? $node->host : HOMER_HOST;
        $mynodes[$node->id]->dbport = isset($node->dbport) ? $node->dbport : HOMER_PORT;
        $mynodes[$node->id]->dbusername = isset($node->dbusername) ? $node->dbusername : HOMER_USER;
        $mynodes[$node->id]->dbpassword = isset($node->dbusername) ? $node->dbpassword : HOMER_PW;

}


/* SECURITY LEVEL: 1 - Admin, 2 - Manager, 3 - User, 4 - Guest*/
$components = array("dashboard"=>ACCESS_DASHBOARD, "search" => ACCESS_SEARCH, "toolbox" => ACCESS_TOOLBOX, "statistic" => ACCESS_STATS, "alarm" =>ACCESS_ALARM, "admin" => ACCESS_ADMIN, "account" => ACCESS_ACCOUNT);

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
  if((!defined('SKIPCFLOWAUTH') || SKIPCFLOWAUTH == 0) && preg_match('/(cflow.php|pcap.php)$/', $_SERVER['PHP_SELF'])) die('Login at first');
	$component = "login";	
	$security = 1;
}

/* Some extra functions  */
/* function getVar($name, $default, $request, $type) {        
        $val = isset($_REQUEST[$name]) ? $_REQUEST[$name] : $default;        
        $type = strtoupper($type);
        if(strcmp($type,"int") == 0) intval($val);
        else if(strcmp($type,"string") == 0) return strval($val);
        else return $val;
}
*/

function getVar($name, $default, $request, $type) {

	$val = isset($request[$name]) ? $request[$name] : $default;

	$type = strtolower($type);

	#INT
	if(strcmp($type,"int") == 0) {
		return intval($val);
	}
	#Float
	elseif(strcmp($type,"float") == 0) {
		return floatval($val);
	}
	#String
	elseif(strcmp($type,"string") == 0) {
		#Strip slashes
		if(get_magic_quotes_gpc()) {
			$val = stripslashes($val);
		}        
		return strval($val);
	}
	#Datetime
	elseif(strcmp($type,"datetime") == 0) {
		return date("Y-m-d H:i:s", strtotime($val));
	}
	#Date
	elseif(strcmp($type,"date") == 0) {
		return date("Y-m-d H:i:s", strtotime($val));
	}
	#Time
	elseif(strcmp($type,"time") == 0) {
		return date("H:i:s", strtotime($val));
	}
	#Other
	else {
		return $val;
	}
}

function detectIE()
{
    if (isset($_SERVER['HTTP_USER_AGENT']) && preg_match("/(MSIE|Internet Explorer)/", $_SERVER['HTTP_USER_AGENT']))
        return true;
    else
        return false;
}

?>
