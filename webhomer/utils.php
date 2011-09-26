<?php
/*
 *        Homer's Utils Box
 *
*/

include("class.db.php");
$db = new homer();

if($db->logincheck($_SESSION['loggedin'], "logon", "password", "useremail") == false){
        //do something if NOT logged in. For example, redirect to login page or display message.
        header("Location: index.php\r\n");
        exit;
}

$task =  getVar('task', 'search', 'post', 'string');

switch ($task) {

        case 'sipmessage':
                sipMessage();
                break;

        case 'test':
                doTest();
                break;

	case 'livesearch':
                liveSearch();
                break;

}

function doTest() { 

	$var = getVar('var', NULL, '', 'string');
	echo "OK";
	echo $var; 

}

function sipMessage() {


	$id = getVar('id', NULL, '', 'string');

	global $mynodeshost, $db;

        //$table = getVar('table', NULL, '', 'string');
        //$node = sprintf("homer_node%02d.", $tnode);

        $option = array(); //prevent problems

        if($db->dbconnect_homer("localhost")) {

                $query = "SELECT * "
                        ."\n FROM ".HOMER_TABLE
                        ."\n WHERE id=$id limit 1";

                $rows = $db->loadObjectList($query);
        }

        // HTML_adminhomer::displayMessage(&$rows);

        // bypass homer.php and parse message body here

	$row = $rows[0];
	$msgbody = $row->msg;
        $msgbody = preg_replace('/</', "&lt;", $msgbody);
        $msgbody = preg_replace('/>/', "&#62;", $msgbody);
        $msgbody = preg_replace('/\n/', "\n<BR>", $msgbody);
        $msgbody = preg_replace('/'.$row->method.'/', "<font color='red'><b>$row->method</b></font>", $msgbody);
        $msgbody = preg_replace('/'.$row->via_1_branch.'/', "<font color='green'><b>$row->via_1_branch</b></font>", $msgbody);
        $msgbody = preg_replace('/'.$row->callid.'/', "<font color='blue'><b>$row->callid</b></font>", $msgbody);
        $msgbody = preg_replace('/'.$row->from_tag.'/', "<font color='red'><b>$row->from_tag</b></font>", $msgbody);
        
        
	echo $msgbody;
}

function liveSearch() {

	$searchterm = getVar('term', NULL, '', 'string');
	$searchfield = getVar('field', NULL, '', 'string');

	if ($searchterm == NULL) {exit;}
	if ($searchfield == NULL) {$searchfield = 'from_user';}

	$return_arr = array();

        global $mynodeshost, $db;

        $option = array(); //prevent problems

        if($db->dbconnect_homer("localhost")) {

                $query = "SELECT distinct ".$searchfield
                        ."\n FROM ".HOMER_TABLE
                        ."\n WHERE ".$searchfield." like '%".$searchterm."%' limit 5";

                $rows = $db->loadObjectList($query);
        }

	// DEBUG
	//print_r($rows);

	foreach($rows as $row) {
		$result = $row->$searchfield;
		if ( substr( $result, 0, 1 ) == "+" ) { $result = substr( $result, 1 ); }
		array_push($return_arr, $result);
	}

	echo json_encode($return_arr);

}



?>
