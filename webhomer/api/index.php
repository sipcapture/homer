<?php
/*
 * HOMER Web Interface
 * Homer's index.php
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
require_once("../preferences.php");

date_default_timezone_set(HOMER_TIMEZONE);
ini_set('date.timezone', HOMER_TIMEZONE);


/* MAIN CLASS modules */
require_once("../class/index.php");
require_once("api/class/restutils.php");
require_once("api/class/process.php");
require_once("api/class/search.php");


//if((!defined("SKIPAUTH") || $component != "login") && $auth->logincheck() == false){
if($auth->logincheck() == false){

      $answer['sid'] = session_id();
      $answer['auth'] = 'false';                                
      $answer['status'] = 'wrong-session';
      sendResponse($reply, json_encode($answer),'application/json');
      
      exit;
}
                  




$obj = processRequest();

/* Call Search */
$search = new Search;
$search->setDB($db);

/* ANSWER */
$answer = array();
$answer['server']     = "apiserver";
$answer['language']   = "en";
$reply = 200;
$method = $obj->getMethod();
$request = $obj->getRURI();
$data = $obj->getData();

if($method == "get" || $method == "post") {

        $ret = basicFeautures($request, $data);        
        $answer = array_merge($answer, $ret);                                            
}
else if($method == "delete") {

        switch($obj->getRURI()) {

                case 'session':
                        $auth->logOut();
                        $answer['status'] = 'ok';
                        break;

                default:
                        $reply = 404;                                        
                        $answer['status'] = 'fail';                                                
                        $answer['message'] = 'Unknown delete call: ['.$obj->getRURI().']';                
                        break;        
        }
}
else {

     $answer['status'] = 'fail';                        
     $answer['message'] = 'Unknown method call: ['.$method.']';
     $reply = 404;
}

/* Total records */
if(isset($answer['data'])) $answer['totalrecords'] = count($answer['data']);
else $answer['totalrecords'] = 0;

sendResponse($reply, json_encode($answer),'application/json');


function basicFeautures($type, $data) {

        global $obj, $auth, $search, $db;
        
        $answer = array();

        switch($type) {

                case 'session':
                
                        if(!isset($data->username) || !isset($data->password)) {
                                if($auth->checkSession()) {
                                    $answer['auth'] = 'true';
                                    $answer['status'] = 'ok';
                                    $answer['sid'] = session_id();
                                }
                                else {                        
                                    $answer['sid'] = session_id();
                                    $answer['auth'] = 'false';                                
                                    $answer['status'] = 'wrong-session';
                                }
                        }
                        else {                                                                                        
                                $answer['user'] = $auth->logIn($data);
                                if(empty($answer['user'])) {
                                    $answer['message'] = 'bad password or username';
                                    $answer['status'] = 'wrong-credentials';                                
                                }                   
                                else {
                                    $answer['status'] = 'ok';
                                }
                        }
                        
                        break;


               case 'user':               

                       $answer['user'] = $auth->getUser();
                       if(count($answer['user'])) $answer['status'] = 'ok';
                       else $answer['status'] = 'no-auth';                        
                       break;                        
                                      

                /* SEARCH CALLS */    

                case 'search/messages/all':
  
			include('DataTable/Autoloader.php');
			spl_autoload_register(array('DataTable_Autoloader', 'autoload'));

			include_once('DataTable/sip/SipDataTable.php');
			include_once('DataTable/sip/SipSearchService.php');

			$sipService = new SipSearchService($db->hostname_homer);
			$table = new SipDataTable();
			$table->setBrowserService($sipService);

			$tmpdata = array();
			foreach($data as $k=>$v) {
			    $tmpdata[$v->name] = $v->value;
			}
			
			$dt_request = new DataTable_Request();
			$dt_request->fromPhpRequest($tmpdata);
			echo $table->renderJson($dt_request);
			exit;
			break;
                
                /* messages */

                case 'message/all/last':
                      if($resp = $search->showMessageAll($data, 0)) {
                              $answer['status'] = 'ok';
                              $answer['data'] = $resp;
                      }
                      else {
                              $answer['status'] = 'not good';
                              $answer['data'] = array();
                      }                      

                      break;

                /* statistic */
                
                case 'statistic/method/all':
                case 'statistic/method/total':
                      $reqkey = preg_replace('/statistic\/method\//', '', $obj->getRURI());                      
                      if($reqkey == "all") $total =0;
                      else $total = 1;
                      
                      if($resp = $search->statisticMethod($data, $total)) {
                              $answer['status'] = 'ok';
                              $answer['data'] = $resp;
                      }
                      else {
                              $answer['status'] = 'not good';
                              $answer['data'] = array();
                      }                      

                      break;

                case 'statistic/data/all':
                case 'statistic/data/total':

                      $reqkey = preg_replace('/statistic\/data\//', '', $obj->getRURI());                      

                      if($reqkey == "all") $total =0;
                      else $total = 1;                
                      
                      if($resp = $search->statisticData($data, $total)) {
                              $answer['status'] = 'ok';
                              $answer['data'] = $resp;
                      }
                      else {
                              $answer['status'] = 'not good';
                              $answer['data'] = array();
                      }                      

                      break;

                case 'statistic/ip/all':
                case 'statistic/ip/total':

                      $reqkey = preg_replace('/statistic\/ip\//', '', $obj->getRURI());                      

                      if($reqkey == "all") $total=0;
                      else $total = 1;                
                      
                      if($resp = $search->statisticIP($data, $total)) {
                              $answer['status'] = 'ok';
                              $answer['data'] = $resp;
                      }
                      else {
                              $answer['status'] = 'not good';
                              $answer['data'] = array();
                      }                      

                      break;



                case 'statistic/useragent/all':
                case 'statistic/useragent/total':

                      $reqkey = preg_replace('/statistic\/useragent\//', '', $obj->getRURI());                      

                      if($reqkey == "all") $total =0;
                      else $total = 1;                
                      
                      if($resp = $search->statisticUserAgent($data, $total)) {
                              $answer['status'] = 'ok';
                              $answer['data'] = $resp;
                      }
                      else {
                              $answer['status'] = 'not good';
                              $answer['data'] = array();
                      }                      

                      break;


                case 'alarm/data/all':
                case 'alarm/data/total':
                case 'alarm/data/short':

                      $reqkey = preg_replace('/alarm\/data\//', '', $obj->getRURI());                      

                      if($reqkey == "all") $total =0;
                      else if($reqkey == "short") $total = 2;
                      else $total = 1;                
                      
                      if($resp = $search->alarmData($data, $total)) {
                              $answer['status'] = 'ok';
                              $answer['data'] = $resp;
                      }
                      else {
                              $answer['status'] = 'not good';
                              $answer['data'] = array();
                      }                      

                      break;


                case 'alarm/update':

                      if($resp = $search->replaceAlarm($data)) {
                              $answer['status'] = 'ok';
                              $answer['data'] = $resp;
                      }
                      else {
                              $answer['status'] = 'not good';
                              $answer['data'] = array();
                      }                      

                      break;

                case 'alarm/delete':

                      if($resp = $search->deleteAlarm($data)) {
                              $answer['status'] = 'ok';
                              $answer['data'] = $resp;
                      }
                      else {
                              $answer['status'] = 'not good';
                              $answer['data'] = array();
                      }                      

                      break;

		case 'export/pcap/callid':
		case 'export/text/callid':
                      include_once("class/pcap.php");
                      /* Call Search */
                      $export = new Export;
                      $export->setDB($db); 
                      $text = 0;                    
                      if($obj->getRURI() == "export/text/callid") $text = 1;
                      
                      $resp = $export->generatePcap($data, $text);
                      if(!empty($resp)){
                              sendFile(200, $resp[0], $resp[1], $resp[2]);
                              exit;
                      }
                      else {
                            $answer['status'] = 'ok';
                            $answer['data'] = array();
                            exit;
                      }
                      break;

                /* ALARMS */


                default: 
                    $answer['status'] = 'not good';
                    $answer['message'] = 'Unknown get call: ['.$request.']';
                    $answer['data'] = array();
                    break;                                            
        }


        return $answer;
}


?>
