<?php
/*
 * HOMER Web Interface
 * Homer's search.php
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

date_default_timezone_set(HOMER_TIMEZONE);

require_once ('search.html.php');

class Component {


          function executeComponent() {
                    
                    global $task;                    
                    
                    //Go if all ok
                    switch ($task) {
                    
                          case 'search':
                              $this->showSearchForm(0);
                              break;            	
		
                          case 'result':
                              $this->showResultSearch();
                              break;
        	                  		
                          case 'showmessage':
                              $this->showMessage();
                              break;

                          case 'showcallflow':
                              $this->showCallFlow();
                              break;                              
             
                          default:
                 	      $this->showSearchForm(0);
                   	      break;    
                   }
          }

          function showSearchForm($type = null) {
        
                  global $mynodesname;
                  $search = array();

                  if(isset($_SESSION['homersearch'])) {
                        $search = json_decode($_SESSION['homersearch'], true);
                  }

                  if($type) HTML_search::displayAdvanceSearchForm($search, $mynodesname);
                  else HTML_search::displaySearchForm($search, $mynodesname);
          }

          function showResultSearch() {

                  global $mynodeshost, $db;
        
                  /* AJAXTYPE FIX */
                  if(!defined('AJAXTYPE')) define('AJAXTYPE', "GET");
                          
                  include('DataTable/Autoloader.php');
                  spl_autoload_register(array('DataTable_Autoloader', 'autoload'));
                  include('DataTable/sip/SipDataTable.php');
                  // instantiate the DataTable

                  $datatable = new SipDataTable();
                  // set the url to the ajax script         
                  $datatable->setAjaxDataUrl(APILOC.'search/messages/all');                                                                                                                        
                                
                  $userparam = new stdclass();
                  $callparam = new stdclass();
                  $headerparam = new stdclass();
                  $timeparam = new stdclass();
                  $networkparam = new stdclass();
        
                  //User
                  $search['ruri_user'] = $userparam->ruri_user = getVar('ruri_user', NULL, $_REQUEST, 'string');
                  $search['to_user'] = $userparam->to_user = getVar('to_user', NULL, $_REQUEST, 'string');
                  $search['from_user'] = $userparam->from_user = getVar('from_user', NULL, $_REQUEST, 'string');
                  $search['pid_user'] = $userparam->pid_user = getVar('pid_user', NULL, $_REQUEST, 'string');
                  $search['contact_user'] = $userparam->contact_user = getVar('contact_user', NULL, $_REQUEST, 'string');
                  $search['auth_user'] = $userparam->auth_user = getVar('auth_user', NULL, $_REQUEST, 'string');
                  $search['logic_or'] = $dbic_or = getVar('logic_or', 0, $_REQUEST, 'int');
	
                  //Call	
                  $search['callid'] = $callparam->callid = getVar('callid', NULL, $_REQUEST, 'string');
                  $search['b2b'] = $b2b = getVar('b2b', 0, $_REQUEST, 'int');		
                  $search['from_tag'] = $callparam->from_tag = getVar('from_tag', NULL, $_REQUEST, 'string');
                  $search['to_tag'] = $callparam->to_tag = getVar('to_tag', NULL, $_REQUEST, 'string');
                  $search['via_1_branch'] = $callparam->via_1_branch = getVar('via_1_branch', NULL, $_REQUEST, 'string');
                  $search['method'] = $callparam->method = getVar('method', NULL, $_REQUEST, 'string');
                  $search['reply_reason'] = $callparam->reply_reason = getVar('reply_reason', NULL, $_REQUEST, 'string');
	
                  //Header
                  $search['ruri'] = $headerparam->ruri = getVar('ruri', NULL, $_REQUEST, 'string');
                  $search['via_1'] = $headerparam->via_1 = getVar('via_1', NULL, $_REQUEST, 'string');
                  $search['diversion'] = $headerparam->diversion = getVar('diversion', NULL, $_REQUEST, 'string');
                  $search['cseq'] = $headerparam->cseq = getVar('cseq', NULL, $_REQUEST, 'string');
                  $search['reason'] = $headerparam->reason = getVar('reason', NULL, $_REQUEST, 'string');
                  $search['content_type'] = $headerparam->content_type = getVar('content_type', NULL, $_REQUEST, 'string');
                  $search['authorization'] = $headerparam->authorization = getVar('authorization', NULL, $_REQUEST, 'string');
                  $search['user_agent'] = $headerparam->user_agent = getVar('user_agent', NULL, $_REQUEST, 'string');
                  $search['msg'] = $headerparam->msg = getVar('msg', NULL, $_REQUEST, 'msg');
	
                  //Time
                  $search['location'] = $location = getVar('location', array(0), $_REQUEST, 'array');	
                  $search['from_date'] = $timeparam->from_date = getVar('from_date', '', $_REQUEST, 'string');	        
                  $search['to_date'] = $timeparam->to_date = getVar('to_date', '', $_REQUEST, 'string');	        
                  $search['from_time'] = $timeparam->from_time = getVar('from_time', NULL, $_REQUEST, 'string');
                  $search['to_time'] = $timeparam->to_time = getVar('to_time', NULL, $_REQUEST, 'string');
                  //$search['max_records'] = $timeparam->max_records = getVar('max_records', 100, $_REQUEST, 'int');
                  $search['unique'] = $unique = getVar('unique', 0, $_REQUEST, 'int');

                  $ft = date("Y-m-d H:i:s", strtotime($timeparam->from_date." ".$timeparam->from_time));
                  $tt = date("Y-m-d H:i:s", strtotime($timeparam->to_date." ".$timeparam->to_time));	
                  $fhour = date("H", strtotime($timeparam->from_date." ".$timeparam->from_time));
                  $thour = date("H", strtotime($timeparam->to_date." ".$timeparam->to_time));

	        
                  //Network	        	
                  $search['source_ip'] = $networkparam->source_ip = getVar('source_ip', NULL, $_REQUEST, 'string');	
                  $search['source_port'] = $networkparam->source_port = getVar('source_port', 0, $_REQUEST, 'int');
                  $search['destination_ip'] = $networkparam->destination_ip = getVar('destination_ip', NULL, $_REQUEST, 'string');	
                  $search['destination_port'] = $networkparam->destination_port = getVar('destination_port', 0, $_REQUEST, 'int');
                  $search['contact_ip'] = $networkparam->contact_ip = getVar('contact_ip', NULL, $_REQUEST, 'string');	
                  $search['contact_port'] = $networkparam->contact_port = getVar('contact_port', 0, $_REQUEST, 'int');
                  $search['originator_ip'] = $networkparam->originator_ip = getVar('originator_ip', NULL, $_REQUEST, 'string');	
                  $search['originator_port'] = $networkparam->originator_port = getVar('originator_port', 0, $_REQUEST, 'int');

                  /* Capture node ID. HEPv2 protocol */
                  $search['node'] = $node = getVar('node', NULL, $_REQUEST, 'string');                  
                  $search['limit'] = $limit = getVar('limit', 0, $_REQUEST, 'int');
                  
                  //Please change protocol
                  //$search['proto'] = $proto = getVar('proto', 2, $_REQUEST, 'int');	
                  //$search['family'] = $family = getVar('family', 2, $_REQUEST, 'int');	
                  $_SESSION['homersearch'] = json_encode($search);
                  

                  if(SEARCHLOG) {
                        $query = $db->makeQuery("INSERT INTO homer_searchlog SET `useremail`='?', `date`=NOW(), `search`= '?';" ,
                                        $_SESSION['loggedin'], $_SESSION['homersearch']);
                        $db->executeQuery($query);                        
                        $query = $db->makeQuery("DELETE FROM homer_searchlog WHERE `date` < (CURDATE() - INTERVAL ".SEARCHLOG." DAY)");
                        $db->executeQuery($query);
                  }
                  
 									/* Since the search array is sent down to the page, and back as an ajax request
 									 * we must urlencode() any of our search parameters which may contain special (url) chars.
 									 * Take note that this encoding must be AFTER storing the search data to the session, or the
 									 * users session data may become corrupt with multiple levels of encoding as they browse around the site.
 									 * Travis Hegner
 									 */
									$search['ruri_user'] = urlencode($search['ruri_user']);
									$search['to_user'] = urlencode($search['to_user']);
									$search['from_user'] = urlencode($search['from_user']);
									$search['pid_user'] = urlencode($search['pid_user']);
									$search['contact_user'] = urlencode($search['contact_user']);

                  $datatable->setSearchRequest($search);
                  
                  $columns = $datatable->getColumns();
                  foreach($columns as $key=>$column) {
                          $visible = $column->isVisible();
                          $title = $column->getTitle();
                          $showColumns[$key]= array("title" => $title, "visible" => $visible);
                  }

                  if (get_magic_quotes_gpc() == true && isset($_COOKIE['homerdata_SIPTable_index_php'])) {
                        $cookie = stripslashes($_COOKIE['homerdata_SIPTable_index_php']);
                        $allcokie = json_decode($cookie);
                        $abvis = $allcokie->abVisCols;
                        foreach($abvis as $key=>$value) $showColumns[$key]["visible"] = $value;
                  }
                  
                  HTML_search::displayResultSearch($datatable, $ft, $tt, $search, $showColumns);
        }

        function showMessage()  {

                global $mynodeshost, $db;
        	$myrows = array();

                $userid = $user->id;
                
                $tnode = getVar('tnode', NULL, $_REQUEST, 'string');
                $location_str = getVar('location', NULL, $_REQUEST, 'string');
                $location = explode(",", $location_str);

                $id = getVar('id', 0, $_REQUEST, 'int');

                //$node = sprintf("homer_node%02d.", $tnode);

                $option = array(); //prevent problems

                if($db->dbconnect_homer(NULL)) {                
                      $query = "SELECT * FROM ".HOMER_TABLE." WHERE id=$id limit 1";
                      $rows = $db->loadObjectList($query);
                }

                HTML_search::displayMessage($rows);                
        }
                                
        function myLocalRedirect( $url='') {
                echo "<script>location.href='$url';</script>\n";
                exit();
        }
}

?>
