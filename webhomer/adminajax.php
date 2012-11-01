<?php
/*
 * HOMER Web Interface
 * Homer's Admin ajax
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


define('_HOMEREXEC', "1");

/* MAIN CLASS modules */
include("class/index.php");

/* Only admin */
$level =  $_SESSION['userlevel'];
if($level != 1) exit;

$type=$_REQUEST['type'];
$tmp = $_REQUEST['categoryId'];

$task  = array("user","host","node");
$tables  = array("logon","hosts","nodes");
$types = array("userlevel","useremail","host","name","status","password","dbname", "dbport", "dbusername", "dbpassword","dbtables");
$adminGroups = array("Admin" => 1, "Manager" => 2, "User" => 3, "Guest" => 4);
        
foreach($task as $key=>$value) {                
      if(preg_match("/^$value(\d+)/",$tmp)) {
              $table="homer_".$tables[$key];
              $catid = intval(substr($tmp,strlen($value)));        
      }             
}

if(!isset($table)) {return 0; exit;}

$data=$_REQUEST['data'];

if($table == "homer_logon") $field_id="userid";
else $field_id="id";

if($type == "remove") {
     $db->qry("delete from ".$table." WHERE {$field_id} = '?';",$catid);
     echo 1;
     exit;
}

/*fix. I'll change it to array */
foreach($types as $key=>$value) { if($value == $type) { $field = $value; break; }}
if(!isset($field)) { return 0; exit;}

/* check groups */
if($field == "userlevel" && $table=="homer_logon") {
        if(isset($adminGroups[$data])) $data = $adminGroups[$data];
        else {return 0; exit;}        
}
else if($field == "password") {
     $query = $db->makeQuery("SELECT * FROM ".$table." WHERE password = '?' AND $field_id = '?';" , $data, $catid);
     if($db->loadResult($query)) {echo 1; exit;}    
     $data = md5($data);
}

$query = $db->makeQuery("update ".$table." SET ".$field."='?' WHERE $field_id = '?';" , $data, $catid);
$db->executeQuery($query);
echo 1;
