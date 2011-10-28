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

	case 'saveit':
                SaveCflow();
                break;

}

function doTest() { 

	$var = getVar('var', NULL, '', 'string');
	echo "OK";
	echo $var; 

}

function sipMessage() {

	      $id = getVar('id', 0, '', 'int');

	      global $mynodeshost, $db;

        $protos = array("UDP","TCP","TLS","SCTP");
        $family = array("IPv4", "IPv6");
        $types = array("REQUEST", "RESPONSE");

        $option = array(); //prevent problems

        if($db->dbconnect_homer(HOMER_HOST)) {

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
        
        unset($row->msg);

?>

<script type="text/javascript">
$(document).ready(function() {
  $('#sipdetails').hide();
  $('input:button').button();
  return false;
});
</script>
<p>
    <input type="button" value="Toggle message" onclick="$('#sipmsg').toggle(400);"  style="background: transparent;" />
    <input type="button" value="Toggle details" onclick="$('#sipdetails').toggle(400);"  style="opacity: 1; background: transparent;"/>

</p>
<div id="sipdetails" style="display: none;">
             <table border="0" cellspacing="2" cellpadding="2"  class="bodystyle">
<?php
                        foreach ($row as $key=>$value) {

                                $value = preg_replace('/</', "&lt;", $value);
                                $value = preg_replace('/>/', "&#62;", $value);
                                if($value == "") continue;
                                $value = preg_replace('/\n/', "\n<BR>", $value);
                                if($key == "proto") $value=$protos[($value-1)];
                                else if($key == "family") $value=$family[($value-2)];
                                else if($key == "type") $value=$types[($value-1)];
                                else if(preg_match("/_port/i", $key) && $value==0) continue; //Skip 0 port
?>
                        <tr >
                            <td align="right" class="dataTableContentBB"><b><?php echo $key;?></b></td>
                            <td align="left" class="dataTableContentBB"><font color="#000"><b><?php echo $value;?></b></font></td>
                         </tr>
<?php
                        }
?>
             </table>
</div>
<div id='sipmsg'>
        <?php echo $msgbody;?>
</div>
<?php
}

function liveSearch() {

  if (AUTOCOMPLETE != 0) {

	$searchterm = getVar('term', NULL, '', 'string');
	$searchfield = getVar('field', NULL, '', 'string');
  	// timedate limit
        $search['date'] = $timeparam->date = getVar('date', '', '', 'string');
        $search['from_time'] = $timeparam->from_time = getVar('from_time', NULL, '', 'string');
        $search['to_time'] = $timeparam->to_time = getVar('to_time', NULL, '', 'string');

        $ft = date("Y-m-d H:i:s", strtotime($timeparam->date." ".$timeparam->from_time));
        $tt = date("Y-m-d H:i:s", strtotime($timeparam->date." ".$timeparam->to_time));
        $fhour = date("H", strtotime($timeparam->date." ".$timeparam->from_time));
        $thour = date("H", strtotime($timeparam->date." ".$timeparam->to_time));
        $j=$thour+1;

        $where = "(`date` >= '$ft' AND `date` <= '$tt' )";

	if ($searchterm == NULL) {exit;}
	if ($searchfield == NULL) {$searchfield = 'from_user';}

	$return_arr = array();

        global $mynodeshost, $db;

        $option = array(); //prevent problems

        if($db->dbconnect_homer(HOMER_HOST)) {

                $query = "SELECT distinct ".$searchfield
                        ."\n FROM ".HOMER_TABLE
                        ."\n WHERE ".$searchfield." like '%".$searchterm."%' and ".$where." limit 5";

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
}

function SaveCflow() {

	if ($_REQUEST['cflow'] != "") { 
	header("Content-type: application/x-cflow-png");
	header("Content-Disposition: attachment; filename=HOMER_CFLOW_".$_REQUEST['cflow']);
	readfile(PCAPDIR.$_REQUEST['cflow']);
	}

}


?>
