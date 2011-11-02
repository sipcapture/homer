<?php
include("class.db.php");
$db = new homer();

if($db->logincheck($_SESSION['loggedin'], "logon", "password", "useremail") == false){
  //do something if NOT logged in. For example, redirect to login page or display message.
  header("Location: index.php\r\n");
  exit;
}
/* Only admin */
$level =  $_SESSION['userlevel'];
if($level != 1) exit;

$type=$_REQUEST['type'];
$tmp = $_REQUEST['categoryId'];

$tid = intval(substr($tmp, 0, 1));
$catid = intval(substr($tmp, 1));
$data=$_REQUEST['data'];

if($tid==3) $table="homer_nodes";
else if($tid==2) $table="homer_hosts";
else if($tid==1) $table="homer_logon";
else { return 0; exit;}

/*fix. I'll change it to array */
if($type == "userlevel") $field="userlevel";
else if($type == "useremail") $field="useremail";
else if($type == "password") $field="password";
else if($type == "host") $field="host";
else if($type == "name") $field="name";
else if($type == "status") $field="status";
else if($type == "remove") $remove=1;
else { return 0; exit;}

if($table == "homer_logon") $field_id="userid";
else $field_id="id";

if($remove) {
     $db->qry("delete from ".$table." WHERE {$field_id} = '?';",$catid);
     echo 1;
     exit;
}

if($field == "password") {
     $result = $db->qry("SELECT * FROM ".$table." WHERE password = '?' AND $field_id = '?';" , $data, $catid);
     if(mysql_num_rows($result)) { echo 1; exit; }     
     $data = md5($data);
}

$db->qry("update ".$table." SET ".$field."='?' WHERE $field_id = '?';" , $data, $catid);
                        
echo 1;
