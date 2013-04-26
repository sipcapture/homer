<?php
/*
 * HOMER Web Interface
 * administrator component
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

require_once ('admin.html.php');

class Component {


          function executeComponent() {
                    
                    global $task;                    
                    //Go if all ok
                    switch ($task) {
                    
                    case 'adminoverview':
                        $this->showNewAdminOverView();
                        break;

                    case 'createuser':
                        $this->showCreateUser();
                         break;          
               
                    case 'createnode':
                         $this->showCreateNode(1);
                         break;

                    case 'createhost':
                         $this->showCreateNode(2);
                         break;               
               
                    default:
          	        $this->showNewAdminOverView(0);
	        	break;    
                }
          }

          function showNewAdminOverView() {

                  global $mynodeshost, $db, $task;
                  
                  $option = array(); //prevent problems


                  // get users
                  $table = "homer_logon";
                  $name_users = "User details";
                  $dval_users = "user";
	
                  $query = "SELECT * FROM ".$table;
                  $rows_users = $db->loadObjectList($query);

                  // get hosts
                  $table = "homer_hosts";
                  $name_hosts = "Hosts details";
                  $dval_hosts = "host";
                  $query = "SELECT * FROM ".$table;
                  $rows_hosts = $db->loadObjectList($query);

                  // get nodes
                  $table = "homer_nodes";
                  $name_nodes = "Nodes details";
                  $dval_nodes = "node";

                  $query = "SELECT * FROM ".$table;
                  $rows_nodes = $db->loadObjectList($query);

                  //print_r($rows);
                  $allrows= array($rows_users, $rows_hosts, $rows_nodes);
                  $allnames= array($name_users, $name_hosts, $name_nodes);
                  $alldval= array($dval_users, $dval_hosts, $dval_nodes);
                  
                  HTML_Admin::displayNewAdminOverView($allrows, $allnames, $task, $alldval);


		  /* USERS/HOSTS/NODES  FORM */
		  $option  = array("logon","hosts","nodes");
		  $headers  = array("USERS","ALIASES","NODES");
 		  $tasks  = array("user","host","node");
  	          $adminGroup = array("Admin","Manager","User","Guest");
 
		  foreach($option as $key=>$value) {
                	$query = "SELECT * FROM homer_".$value;
	                $rows = $db->loadObjectList($query);
			$datas[]=$rows;
        	        $names[]=$value;
                	$types[] = $tasks[$key];
	                /* HEADER */
		  }
                                  
                  HTML_Admin::displayAdminUsers($datas,$names,$types);

                  HTML_Admin::displayAdminInfo();                  

          	  $report = $this->server_report();
                 
                  HTML_Admin::displayAdminHealth($report);
                  
		  if (ADMIN_NETSTAT != 0) {
          	  $bwstats = $this->network_report();
                  HTML_Admin::displayNetworkStats($bwstats);
		  }                         
		  if (ADMIN_DBSTAT != 0) {
          	  $bwstats = $this->database_report();
                  HTML_Admin::displayDBStats($bwstats);
		  }                         

                  HTML_Admin::displayAdminForms();
                  
          }
          
          
          function check_port($port) {
		    if ($port == 0) { return true; }
		    $conn = @fsockopen("127.0.0.1", $port, $errno, $errstr, 0.2);
		    if ($conn) {
		        fclose($conn);
		        return true;
		    } 
          }

	  function check_port_udp($port) {
		    if ($port == 0) { return true; }
                    $conn = @fsockopen("udp://127.0.0.1", $port, $errno, $errstr);
                    if ($conn) {
                        fclose($conn);
                        return true;
                    } 
          }

	  function check_service_local($service) {
		  if ($service == "SIPCAPTURE") $service_name="-e kamailio.pid -e opensips.pid";
		  if ($service == "MySQL") $service_name="-e mysqld -e postgres";
		    $check = exec('ps aux | grep -v grep | grep '.$service_name);
		    if ($check) {
			return true;
		    }
	  }

	  function server_report() {
		   // get service definitions from preferences 
		   if (!defined('HOMER_PORT')) { $sql_port='3306'; } else { $sql_port = HOMER_PORT; }
                   if (!defined('SERVICE_HTTP_PORT')) { $http_port='80'; }  else { $http_port = SERVICE_HTTP_PORT; }
                   if (!defined('SERVICE_SMTP_PORT')) { $smtp_port='25'; }  else { $smtp_port = SERVICE_SMTP_PORT; }
                   if (!defined('SERVICE_SSH_PORT'))  { $ssh_port='22'; }   else { $ssh_port = SERVICE_SSH_PORT; }
                   if (!defined('SERVICE_SIP_PORT'))  { $sip_port='5060'; } else { $sip_port = SERVICE_SIP_PORT; }

		    $report = array();
		    $tcp_serv = array(
				  $sql_port =>'MySQL',
		                  $http_port =>'HTTP',
		                  $smtp_port =>'SMTP',
		                  $ssh_port =>'SSH');
		    $udp_serv = array(
                                  $sip_port =>'SIPCAPTURE');


		    foreach ($udp_serv as $port=>$service) {
			// UDP checks are always false positives
                        //$report[$service] = check_port_udp($port);
                        $report[$service] = $this->check_service_local($service);
                    }
		    foreach ($tcp_serv as $port=>$service) {
		        $report[$service] = $this->check_port($port);
		    }

		    return $report;
          }


	  function database_report() {

		global $mynodeshost, $db;

		$dbstats = array();
		$query = 'SHOW STATUS';
                $dbstat = $db->loadObjectList($query);
		$qps=$dbstat->Questions->Value;
		foreach ($dbstat as $row) {
			if ($row->Value != 0) {
				if(!preg_match("/HOMER_|Innodb|Com_|Flush_/", $row->Variable_name)) {
					//echo $row->Variable_name . ' = ' . $row->Value . "<br>";
					$dbstats[$row->Variable_name] =  $row->Value;
				}
			}
		}

		return $dbstats;

	  }

          
	  function network_report() {

		// grab some basic network vars
		$bwstats = array();
	        exec('cat /proc/net/dev|grep eth',$bufr);
	            foreach ($bufr as $buf) {
                    	list($dev_name, $stats_list) = preg_split('/:/', $buf, 2);
                    	$stats = preg_split('/\s+/', trim($stats_list));
                        $bwstats["TX PACKETS"] += $stats[8];
                        $bwstats["RX PACKETS"] += $stats[0];
                        $bwstats["ERRORS"] += ($stats[2] + $stats[10]);
                        $bwstats["DROPPED"] += ($stats[3] + $stats[11]);
            	        }
		 	exec('bwm-ng -o plain -c 1 | grep eth', $result, $status);
		        if ($status != 0) {
			        $bwstats[RATE] = "Not Available. Install bwn-ng";
		        } else {
				foreach ($result as $result2) {
                    		$cstats = preg_split('/  +/', trim($result2));
		        	$bwstats["RATE ".$cstats[0]] = $cstats[3]." (TX:".$cstats[1]." RX: ".$cstats[2].")";
			}
		    }

		return $bwstats;
	  }

          function showCreateUser() {

                  global $mynodeshost, $db, $task, $component;

                  $userid = $user->id;
                  $returntask = getVar('returntask', NULL, $_REQUEST, 'string');
                  $email = getVar('email', NULL, $_REQUEST, 'string');
                  $password = getVar('password', NULL, $_REQUEST, 'string');
                  $level = getVar('level', 1, $_REQUEST, 'int');	                         		
                  $db->qry("INSERT into homer_logon set useremail='?', password='?', userlevel='?'", $email, md5($password), $level);
                  $this->myLocalRedirect("index.php?component={$component}");
                  exit;	
          }

          function showCreateNode($type) {
          
          	global $mynodeshost, $db, $task, $component;
          
          	$userid = $user->id;
          	$returntask = getVar('returntask', NULL, $_REQUEST, 'string');
          	$host = getVar('host', NULL, $_REQUEST, 'string');
          	$name = getVar('name', NULL, $_REQUEST, 'string');
          	$status = getVar('status', 1, $_REQUEST, 'int');
          	$database = getVar('dbname', NULL, $_REQUEST, 'string');
          	$port = getVar('dbport', NULL, $_REQUEST, 'string');
          	$username = getVar('dbusername', NULL, $_REQUEST, 'string');
          	$password = getVar('dbpassword', NULL, $_REQUEST, 'string');
          
          	$table = $type == 1 ? "homer_nodes" : "homer_hosts";
          	if($type==1)
          		$db->qry("INSERT into $table set host='?', name='?', status='?', dbname='?', dbport='?', dbusername='?', dbpassword='?'", $host, $name, $status, $database, $port, $username, $password);
          	else
          		$db->qry("INSERT into $table set host='?', name='?', status='?'", $host, $name, $status);
          	$this->myLocalRedirect("index.php?component={$component}");
          	exit;
          }
                    
	 function myLocalRedirect( $url='') {
                echo "<script>location.href='$url';</script>\n";
                exit();
         }          
}

?>
