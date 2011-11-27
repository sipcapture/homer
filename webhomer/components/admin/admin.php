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

                  HTML_Admin::displayNewAdminOverView($type, $allrows, $allnames, $task, $alldval);


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
                  
                  
                  HTML_Admin::displayAdminForms();
                  
          }
          
          
          function check_port($port) {
		    $conn = @fsockopen("127.0.0.1", $port, $errno, $errstr, 0.2);
		    if ($conn) {
		        fclose($conn);
		        return true;
		    } 
          }

	  function check_port_udp($port) {
                    $conn = @fsockopen("udp://127.0.0.1", $port, $errno, $errstr);
                    if ($conn) {
                        fclose($conn);
                        return true;
                    } 
          }

	  function check_service_local($service) {
		  if ($service == "SIPCAPTURE") $service_name="-e kamailio.pid -e opensips.pid";
		  if ($service == "MySQL") $service_name="mysqld";
		    $check = exec('ps aux | grep -v grep | grep '.$service_name);
		    if ($check) {
			return true;
		    }

	  }

	  function server_report() {
		    // check new config for sql port
		    if (!HOMER_PORT) {$sqlport='3306';} else {$sqlport = HOMER_PORT;}
		    $report = array();
		    $tcp_serv = array(
				  $sqlport =>'MySQL',
		                  '80'=>'HTTP',
		                  '25'=>'SMTP',
		                  '22'=>'SSH');
		    $udp_serv = array(
                                  '5060'=>'SIPCAPTURE');


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
          

          function showCreateUser() {

                  global $mynodeshost, $db, $task, $component;

                  $userid = $user->id;
                  $returntask = getVar('returntask', NULL, '', 'string');
                  $email = getVar('email', NULL, '', 'string');
                  $password = getVar('password', NULL, '', 'string');
                  $level = getVar('level', 1, '', 'int');	                         		
                  $db->qry("INSERT into homer_logon set useremail='?', password='?', userlevel='?'", $email, md5($password), $level);
                  $this->myLocalRedirect("index.php?component={$component}");
                  exit;	
          }

          function showCreateNode($type) {

                  global $mynodeshost, $db, $task, $component;
	
                  $userid = $user->id;
                  $returntask = getVar('returntask', NULL, '', 'string');
                  $host = getVar('host', NULL, '', 'string');
                  $name = getVar('name', NULL, '', 'string');
                  $status = getVar('status', 1, '', 'int');

                  $table = $type == 1 ? "homer_nodes" : "homer_hosts";	                      
	                         		
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
