<?php
include("class.db.php");
$db = new homer();

if($db->logincheck($_SESSION['loggedin'], "logon", "password", "useremail") == false){
  //do something if NOT logged in. For example, redirect to login page or display message.
    header("Location: index.php\r\n");
    exit;
}
      
// register the DataTable autoloader
include('DataTable/Autoloader.php');
spl_autoload_register(array('DataTable_Autoloader', 'autoload'));

// include the Demo DataTable class
include_once('SipDataTable.php');

// build a Browser Service object based on the type that was selected
include_once('SipSearchService.php');
                         
$sipService = new SipSearchService($db->hostname_homer, $db->database_homer, $db->username_homer, $db->password_homer);
  
// instatiate new DataTable
$table = new SipDataTable();

$table->setBrowserService($sipService);

$request = new DataTable_Request();
$request->fromPhpRequest($_REQUEST);

// render the JSON data string
echo $table->renderJson($request);
