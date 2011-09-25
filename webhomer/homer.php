<?php

/*
 *        Author: Alexandr Dubovikov <alexandr.dubovikov@gmail.com>
 *        
*/

include("class.db.php");
$db = new homer();

if($db->logincheck($_SESSION['loggedin'], "logon", "password", "useremail") == false){
	//do something if NOT logged in. For example, redirect to login page or display message.
	header("Location: index.php\r\n");
	exit;
}

define(_HOMEREXEC, "1");
require_once ('homer.html.php');

$task =  getVar('task', 'search', 'post', 'string');

 /* My Nodes */
$mynodeshost = array();
$mynodesname = array();
$nodes = $db->getAliases('nodes');
foreach($nodes as $node) {
        $mynodeshost[$node->id] = $node->host;
        $mynodesname[$node->id] = $node->name;
}
                        
$header = 1;
if(strcmp($task, "search")== 0) $title="Search";
else if(strcmp($task, "advsearch")==0) $title="Advanced Search";
else if(strcmp($task, "result")==0) $title="Result";
else if(strcmp($task, "stats")==0) $title="Stats";
else if(strncmp($task, "admin", 5)==0) $title="Admin";
else {
        $title="message";
        $header = 0;
}

$level =  $_SESSION['userlevel'];

$admin_task = array('adminoverview','adminusers','adminnodes','createuser', 'createhost','createhost');

if($level != 1 && in_array($task,$admin_task)) {
                echo "You don't have permissions";
                exit;
}


HTML_adminhomer::displayStart($title, $header, $task, $level);

//Go if all ok
switch ($task) {

        case 'list':
		showHomerList();
        	break;
        	
	case 'search':
	        showSearchForm(0);
		break;            	
		
	case 'stats':
                showStats();
                break;

	case 'advsearch':
	        showSearchForm(1);
		break;            		

        case 'result':
                showResultSearch();
                break;
        	                  		
       case 'showmessage':
               showMessage();
               break;

       case 'showcallflow':
               showCallFlow();
               break;
               
       case 'adminoverview':
       case 'adminusers':
               showAdminOverView(1);
               break;               

       case 'adminnodes':
               showAdminOverView(2);
               break;                              

       case 'adminhosts':
               showAdminOverView(3);
               break;               
               
       case 'createuser':
               showCreateUser();
               break;          
               
       case 'createnode':
               showCreateNode(1);
               break;

       case 'createhost':
               showCreateNode(2);
               break;               
               
               
       case 'logout':
               logout();
               break;
               
	default:
	        showSearchForm(0);
		break;    
}

HTML_adminhomer::displayStop();



function logout() {

        global $db;
        
        $db->logout();        
	header("Location: index.php\r\n");
	echo "<script>location.href='index.php';</script>\n";
	exit;
}


function showSearchForm($type = null) {
        
        global $mynodesname;

        $search = array();

        if(isset($_SESSION['homersearch'])) {
                $search = json_decode($_SESSION['homersearch'], true);
        }

        if($type) HTML_adminhomer::displayAdvanceSearchForm(&$search, $mynodesname);
        else HTML_adminhomer::displaySearchForm(&$search, $mynodesname);
}


function showResultSearch() {

        global $mynodeshost, $db;
        
        include('DataTable/Autoloader.php');
        spl_autoload_register(array('DataTable_Autoloader', 'autoload'));
        // include the Demo DataTable class
        include('SipDataTable.php');
        // instantiate the DataTable
        $datatable = new SipDataTable();
         // set the url to the ajax script         
        $datatable->setAjaxDataUrl('ajax.php');                                                                                                                        
                
        
        
        $userparam = new stdclass();
        $callparam = new stdclass();
        $headerparam = new stdclass();
        $timeparam = new stdclass();
        $networkparam = new stdclass();
        
        //User
        $search['ruri_user'] = $userparam->ruri_user = getVar('ruri_user', NULL, 'post', 'string');
	$search['to_user'] = $userparam->to_user = getVar('to_user', NULL, 'post', 'string');
	$search['from_user'] = $userparam->from_user = getVar('from_user', NULL, 'post', 'string');
	$search['pid_user'] = $userparam->pid_user = getVar('pid_user', NULL, 'post', 'string');
	$search['contact_user'] = $userparam->contact_user = getVar('contact_user', NULL, 'post', 'string');
	$search['auth_user'] = $userparam->auth_user = getVar('auth_user', NULL, 'post', 'string');
	$search['logic_or'] = $dbic_or = getVar('logic_or', 0, 'post', 'int');
	
	//Call	
	$search['callid'] = $callparam->callid = getVar('callid', NULL, 'post', 'string');
	$search['callid_aleg'] = $callid_aleg = getVar('callid_aleg', 0, 'post', 'int');		
	$search['from_tag'] = $callparam->from_tag = getVar('from_tag', NULL, 'post', 'string');
	$search['to_tag'] = $callparam->to_tag = getVar('to_tag', NULL, 'post', 'string');
	$search['via_1_branch'] = $callparam->via_1_branch = getVar('via_1_branch', NULL, 'post', 'string');
	$search['method'] = $callparam->method = getVar('method', NULL, 'post', 'string');
	$search['reply_reason'] = $callparam->reply_reason = getVar('reply_reason', NULL, 'post', 'string');
	
	//Header
	$search['ruri'] = $headerparam->ruri = getVar('ruri', NULL, 'post', 'string');
	$search['via_1'] = $headerparam->via_1 = getVar('via_1', NULL, 'post', 'string');
	$search['diversion'] = $headerparam->diversion = getVar('diversion', NULL, 'post', 'string');
	$search['cseq'] = $headerparam->cseq = getVar('cseq', NULL, 'post', 'string');
	$search['reason'] = $headerparam->reason = getVar('reason', NULL, 'post', 'string');
	$search['content_type'] = $headerparam->content_type = getVar('content_type', NULL, 'post', 'string');
	$search['authorization'] = $headerparam->authorization = getVar('authorization', NULL, 'post', 'string');
	$search['user_agent'] = $headerparam->user_agent = getVar('user_agent', NULL, 'post', 'string');
	
	//Time
        $search['location'] = $location = getVar('location', array(), 'post', 'array');	
	$search['date'] = $timeparam->date = getVar('date', '', 'post', 'string');	        
	$search['from_time'] = $timeparam->from_time = getVar('from_time', NULL, 'post', 'string');
	$search['to_time'] = $timeparam->to_time = getVar('to_time', NULL, 'post', 'string');
	$search['max_records'] = $timeparam->max_records = getVar('max_records', 100, 'post', 'int');
	$search['unique'] = $unique = getVar('unique', 0, 'post', 'int');

	$ft = date("Y-m-d H:i:s", strtotime($timeparam->date." ".$timeparam->from_time));
	$tt = date("Y-m-d H:i:s", strtotime($timeparam->date." ".$timeparam->to_time));
	
	$fhour = date("H", strtotime($timeparam->date." ".$timeparam->from_time));
	$thour = date("H", strtotime($timeparam->date." ".$timeparam->to_time));

	        
	//Network	        	
	$search['source_ip'] = $networkparam->source_ip = getVar('source_ip', NULL, 'post', 'string');	
	$search['source_port'] = $networkparam->source_port = getVar('source_port', 0, 'post', 'int');
	$search['destination_ip'] = $networkparam->destination_ip = getVar('destination_ip', NULL, 'post', 'string');	
	$search['destination_port'] = $networkparam->destination_port = getVar('destination_port', 0, 'post', 'int');
	$search['contact_ip'] = $networkparam->contact_ip = getVar('contact_ip', NULL, 'post', 'string');	
	$search['contact_port'] = $networkparam->contact_port = getVar('contact_port', 0, 'post', 'int');
	$search['originator_ip'] = $networkparam->originator_ip = getVar('originator_ip', NULL, 'post', 'string');	
	$search['originator_port'] = $networkparam->originator_port = getVar('originator_port', 0, 'post', 'int');


	$datatable->setSearchRequest($search);
	

	//Please change protocol
	//$search['proto'] = $proto = getVar('proto', 2, 'post', 'int');	
	//$search['family'] = $family = getVar('family', 2, 'post', 'int');	
	
	$_SESSION['homersearch'] = json_encode($search);

        /* My Hosts */		
	$myhosts = array();
	$hosts = $db->getAliases('hosts');	
	foreach($hosts as $host) $myhosts[$host->host] = $host->name;	
	
	/* My Nodes */
	/* $mynodes = array();
	$nodes = $db->getAliases('nodes');	
	foreach($nodes as $node) $mynodes[$node->id] = $node->host;	
	*/

	$j=$thour+1;
	
	//for($i=$fhour; $i < $j; $i++) $table[] = sprintf("homer_%02d_%02d", date("N", strtotime($timeparam->date)), $i);	
	//if table empty, set first hour
	//if(!count($table)) $table[] = sprintf("homer_%02d_%02d", date("N", strtotime($timeparam->date)), $fhour);	
	
	
	//Partition table	
	//$table[] = "homer_part";
	
	HTML_adminhomer::displayResultSearch(&$datatable, $ft, $tt);

}

function showAdminOverView($type) {

        global $mynodeshost, $db, $task;
	
	$userid = $user->id;
	
        $option = array(); //prevent problems
        

        if($type == 1) {
                $table = "homer_logon";
                $name = "User details";
                $dval = "user";
        }
        else if($type == 2) {
                $table = "homer_hosts";
                $name = "Hosts details";
                $dval = "host";
        }
        else {
                $table = "homer_nodes";
                $name = "Nodes details";
                $dval = "node";                
        }



        $query = "SELECT *"
                ."\n FROM ".$table;
        //        ."\n WHERE id=$id limit 1";
        $rows = $db->loadObjectList($query);	

        //print_r($rows);                        

        HTML_adminhomer::displayAdminOverView($type, $rows, $name, $task, $dval);
}

function showCreateUser() {

        global $mynodeshost, $db, $task;
	
	$userid = $user->id;
	$returntask = getVar('returntask', NULL, '', 'string');
	$email = getVar('email', NULL, '', 'string');
	$password = getVar('password', NULL, '', 'string');
	$level = getVar('level', 1, '', 'int');
	                         		
	$db->qry("INSERT into homer_logon set useremail='?', password='?', userlevel='?'", $email, md5($password), $level);
	myLocalRedirect("homer.php?task={$returntask}");
	exit;	
}

function showCreateNode($type) {

        global $mynodeshost, $db, $task;
	
	$userid = $user->id;
	$returntask = getVar('returntask', NULL, '', 'string');
	$host = getVar('host', NULL, '', 'string');
	$name = getVar('name', NULL, '', 'string');
	$status = getVar('status', 1, '', 'int');

        $table = $type == 1 ? "homer_nodes" : "homer_hosts";	                      
	                         		
	$db->qry("INSERT into $table set host='?', name='?', status='?'", $host, $name, $status);
	myLocalRedirect("homer.php?task={$returntask}");
	exit;	
}

function showMessage()  {

        global $mynodeshost, $db;
	$myrows = array();

        $userid = $user->id;

        //$table = getVar('table', NULL, '', 'string');
        $tnode = getVar('tnode', NULL, '', 'string');
        $location_str = getVar('location', NULL, '', 'string');
        $location = explode(",", $location_str);

        $id = getVar('id', 0, '', 'int');

        //$node = sprintf("homer_node%02d.", $tnode);

        $option = array(); //prevent problems

        if($db->dbconnect_homer("localhost")) {

                $query = "SELECT * "
                        ."\n FROM ".HOMER_TABLE
                        ."\n WHERE id=$id limit 1";

                $rows = $db->loadObjectList($query);
        }

        HTML_adminhomer::displayMessage(&$rows);

}



function showCallFlow_deprecated()  {
	
        global $mynodeshost, $db;
        
	//$table = getVar('table', NULL, '', 'string');
	$tnode = getVar('tnode', NULL, '', 'string');
	$unique = getVar('unique', 0, 'post', 'int');
	$tag = getVar('tag', 0, 'post', 'int');
	$tag = 0;
	
	$location_str = getVar('location', NULL, '', 'string');
	$location = explode(",", $location_str);
	        	
	$id = getVar('id', 0, '', 'int');
	//Location
	
	$node = sprintf("homer_node%02d.", $tnode);
	$option = array(); //prevent problems

	if(!$db->dbconnect_homer($mynodeshost[$tnode])) {
	        echo "Homer error connect";	
                return;
	}
			        	                        
        $table = mysql_real_escape_string($table);	        	                        
        $query = "SELECT callid,from_tag "
                ."\n FROM ".HOMER_TABLE
	        ."\n WHERE id=$id limit 1";
        $callids = $db->loadObjectList($query);
        

        $callid = mysql_real_escape_string($callids[0]->callid);
        $from_tag = mysql_real_escape_string($callids[0]->from_tag);
        
        $andtag = "";
        if($tag) $andtag = " AND from_tag = '".$from_tag."'";
	
	#$node = sprintf("homer_node%02d.", $value);	                        	
                
        $query = "(SELECT id, date, micro_ts, source_ip, source_port, destination_ip, destination_port, msg"
                        ."\n FROM ".HOMER_TABLE
	        	."\n WHERE callid='".$callid."' $andtag) order by micro_ts ASC limit 100";	        	
        
        $myrows = array();
        
	foreach($location as $value) {
        	$option = array(); //prevent problems
        	if($db->dbconnect_homer($mynodeshost[$value])) {
                	$rows = $db->loadObjectList($query);					
                	$myrows[$value]=$rows;
                }        	
        }

	$pcap_path = PCAPDIR.$callid;
	
	if(is_dir($pcap_path)) rmdir($pcap_path);
	
	mkdir($pcap_path);

	$i=0;
	$count=100;


	foreach ($myrows as $rows) {
        	foreach ($rows as $row) {
	
        	        //only unique
			if($unique) {
		        	 $md5sum = md5($row->msg);
        	                 if(isset($message[$md5sum]) && $message[$md5sum] != $row->tnode) continue;
				 else $message[$md5sum] = $row->tnode;
			}

	        	$i++;	
	        	$count++;
	        	//Correlation
	        	//if($row->tnode == 2) (int) $row->micro_ts - (int) 4000;
			
        		$fp = fopen($pcap_path.'/message.txt', 'w');		
	        	fwrite($fp, $row->msg);
        		fclose($fp);		
		
	        	$date = date("Y-m-d H:i:s",$row->micro_ts / 1000000);
		
        		$fp = fopen($pcap_path.'/message-hex.txt', 'w');		
        		$mts = (int) $count.substr($row->micro_ts, -5);
	        	fwrite($fp, $date.".".$mts."\n");
        		fclose($fp);		

	        	$execstring = "cat ".$pcap_path."/message.txt | od -A x -t x1 >> ".$pcap_path."/message-hex.txt";
        		exec($execstring);				
	        	$execstring = TEXT2PCAP."  -t '%F %T.' -4 ".$row->source_ip.",".$row->destination_ip." -u ".$row->source_port.",".$row->destination_port." ".$pcap_path."/message-hex.txt ".$pcap_path."/".$i.".pcap 2>&1 > /dev/null";
        		exec($execstring);				
		
	        	$exdata .= $pcap_path."/".$i.".pcap ";
        	}
        }
	$execstring = MERGECAP." -w ".$pcap_path."/trace.pcap ".$exdata;
	exec($execstring);	
	exec("cd ".PCAPDIR.";".CALLFLOW." ".$pcap_path."/trace.pcap");	
	header("Location: ".WEBPCAPLOC.$callid."/trace/index.html");		
	exit;
	
}
                                
function myLocalRedirect( $url='') {
        echo "<script>location.href='$url';</script>\n";
        exit();
}




function getVar($name, $default, $request, $type) {

        $val = $_REQUEST[$name];
        if(!$val) $val = $default;
        $type = strtoupper($type);                
        if(strcmp($type,"int") == 0) intval($val);
        else if(strcmp($type,"string") == 0) return strval($val);
        else return $val;
} 

function showStats() {

        HTML_adminhomer::displayStats();

}



?>
