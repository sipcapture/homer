<?php
/*
 * HOMER Web Interface
 * Homer's Utils Box
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

define(_HOMEREXEC, "1");

/* MAIN CLASS modules */
include("class/index.php");

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

        case 'sipsend':
                phpSip();
                break;

	case 'reconfig':
                // reConf();
                break;
}

function doTest() { 

	$var = getVar('var', NULL, '', 'string');
	echo "OK";
	echo $var; 

}

function sipMessage() {

	$id = getVar('id', 0, '', 'int');
	$popuptype = getVar('popuptype', 1, '', 'int');
  $tnode = getVar('tnode', 0, '', 'int');

	global $mynodeshost, $db;

        $protos = array("UDP","TCP","TLS","SCTP");
        $family = array("IPv4", "IPv6");
        $types = array("REQUEST", "RESPONSE");
 
        //Crop Search Parameters, if any
        $flow_from_date = getVar('from_date', NULL, 'get', 'string');
        $flow_from_time =  getVar('from_time', NULL, 'get', 'string');
 
        if (isset($flow_from_date, $flow_from_time))
        {
          $ft = date("Y-m-d H:i:s", strtotime($flow_from_date." ".$flow_from_time));
          $where = "(`date` = '$ft') AND ";
        }

        $option = array(); //prevent problems

        if($db->dbconnect_homer(isset($mynodeshost[$tnode]) ? $mynodeshost[$tnode] : NULL)) {
        
                $query = "SELECT * "
                        ."\n FROM ".HOMER_TABLE
                        ."\n WHERE ".$where." id=$id limit 1";

                $rows = $db->loadObjectList($query);
        }

        // HTML_adminhomer::displayMessage(&$rows);

        // bypass index.php and parse message body here

	      $row = $rows[0];
	      $msgbody = $row->msg;
        $msgbody = preg_replace('/</', "&lt;", $msgbody);
        $msgbody = preg_replace('/>/', "&#62;", $msgbody);
        $msgbody = preg_replace('/\n/', "\n<BR>", $msgbody);
        $msgbody = preg_replace('/'.$row->method.'/', "<font color='red'><b>$row->method</b></font>", $msgbody);
        $msgbody = preg_replace('/'.$row->via_1_branch.'/', "<font color='green'><b>$row->via_1_branch</b></font>", $msgbody);
        $msgbody = preg_replace('/'.$row->callid.'/', "<font color='blue'><b>$row->callid</b></font>", $msgbody);
        $msgbody = preg_replace('/'.$row->from_tag.'/', "<font color='red'><b>$row->from_tag</b></font>", $msgbody);
        $msgbody = "<br><font size=-2>".$msgbody."</font>";
        unset($row->msg);

	$winid = rand(1111, 9999)."pop";
?>

<?php if($popuptype == 2): ?>
<link href="styles/core_styles.css" rel="stylesheet" type="text/css" />
<link href="styles/form.css" rel="stylesheet" type="text/css" />
<link type="text/css" href="styles/jquery-ui-1.8.4.custom.css" rel="stylesheet" />
<script src="js/jquery-1.6.4.min.js" type="text/javascript"></script>
<script type="text/javascript" src="js/jquery-ui-1.8.16.custom.min.js"></script>
<?php endif; ?>

<script type="text/javascript">
$(document).ready(function() {
  // var mytitle = $(this).find('span.ui-dialog-title').text();
  $('#sipdetails<?php echo $winid; ?>').hide();
  $('input:button').button;
  $(this).parent().height('auto');
  // $(this).find('span.ui-dialog-title').append( $(this).find('#xbuttons') );
  return false;
});
</script>
<title>Message <?php echo $id;?></title>
<div style="margin-left: 15px">
<p>
    <input type="button" value="+/- Message" onclick="$('#sipmsg<?php echo $winid; ?>').toggle(400);"  style="background: transparent;" class="ui-button ui-widget ui-state-default ui-corner-all"/>
    <input type="button" value="+/- Details" onclick="$('#sipdetails<?php echo $winid; ?>').toggle(400);"  style="opacity: 1; background: transparent;" class="ui-button ui-widget ui-state-default ui-corner-all"/>
</p>
<div id="sipdetails<?php echo $winid; ?>" style="display: none;">
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
<div id='sipmsg<?php echo $winid; ?>'>
        <?php echo $msgbody;?>
</div>
</div>
<?php
}

function liveSearch() {

  if (AUTOCOMPLETE != 0) {

        $searchterm = getVar('term', NULL, '', 'string');
        $searchfield = getVar('field', NULL, '', 'string');
        $tnode = getVar('tnode', 0, '', 'int');
        // timedate limit
        $search['date'] = $timeparam->date = getVar('date', '', '', 'string');
        $search['from_date'] = $timeparam->date = getVar('from_date', '', '', 'string');
        $search['to_date'] = $timeparam->date = getVar('to_date', '', '', 'string');
        $search['from_time'] = $timeparam->from_time = getVar('from_time', NULL, '', 'string');
        $search['to_time'] = $timeparam->to_time = getVar('to_time', NULL, '', 'string');

        $ft = date("Y-m-d H:i:s", strtotime($timeparam->from_date." ".$timeparam->from_time));
        $tt = date("Y-m-d H:i:s", strtotime($timeparam->to_date." ".$timeparam->to_time));
	//        $fhour = date("H", strtotime($timeparam->date." ".$timeparam->from_time));
	//        $thour = date("H", strtotime($timeparam->date." ".$timeparam->to_time));
	//        $j=$thour+1;

        $where = "(`date` >= '$ft' AND `date` <= '$tt' )";

	if ($searchterm == NULL) {exit;}
	if ($searchfield == NULL) {$searchfield = 'from_user';}

	$return_arr = array();

        global $mynodeshost, $db;

        $option = array(); //prevent problems

        if($db->dbconnect_homer(isset($mynodeshost[$tnode]) ? $mynodeshost[$tnode] : NULL)) {

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

function phpSip() {
        require_once('php-sip/PhpSIP.class.php');
        $phpsip_to = getVar('to', NULL, '', 'string');
        $phpsip_from = getVar('from', NULL, '', 'string');
        $phpsip_prox = getVar('proxy', NULL, '', 'string');
        $phpsip_meth = getVar('method', NULL, '', 'string');
        $phpsip_head = getVar('head', NULL, '', 'string');
        echo "FROM: ".$phpsip_from."<br>TO: ".$phpsip_to."<br>VIA ".$phpsip_prox."<br>METHOD: ".$phpsip_meth
	."<br>HEAD: ".$phpsip_head."<br>";
        echo "<br>";
        /* Sends test message */
        try
        {
          $api = new PhpSIP();
          $api->setProxy(''.$phpsip_prox);
          $api->addHeader('X-Capture: '.$phpsip_head);
          $api->setMethod(''.$phpsip_meth);
          $api->setFrom("sip:".$phpsip_from);
          $api->setUri("sip:".$phpsip_to);
          $api->setUserAgent('HOMER/Php-Sip');
          $res = $api->send();

          echo "SIP response: $res\n";

        } catch (Exception $e) {

          echo $e;
        }
}

function reConf() {

	$var_from = getVar('from', NULL, '', 'string');
	$var_to = getVar('to', NULL, '', 'string');
	$cfile = "configuration.php";
	if (isset($var_from)) {
	$file = @file_get_contents($cfile);
	if($file) {
	    if (strstr($file,$var_from)) {
	    $file = str_replace($var_from, $var_to, $file);
	    	//print $file;
		file_put_contents($cfile,$file);
		} else { echo "string not found"; }
	} else { echo "no file"; }
	} else { echo "no vars"; }

}


?>
