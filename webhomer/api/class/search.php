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

defined( '_HOMEREXEC' ) or die( 'Restricted access' );

class Search {

    function setDB($db) {                   
            $this->db =  $db;
    }    
    
    function setUUID($uuid, $gid) {                   
            $this->uuid =  $uuid;
            $this->gid =  $gid;
            $this->uuidQuery = " ";
            if($this->gid != 1) $this->uuidQuery = " AND uuid = ".$this->uuid;
            
    }    

    function showMessageAll($param, $type) {

           date_default_timezone_set(HOMER_TIMEZONE);
           
           $mydb = $this->db;                            
           
           $reqdata = (array) $param;           
           $search = array();
	   $callwhere = array();

	   // SEARCH
           $search['method'] = getVar('method', NULL, $reqdata, 'string');           
           $search['ruri_user'] = getVar('ruri_user', NULL, $reqdata, 'string');         
           $search['from_user'] = getVar('from_user', NULL, $reqdata, 'string');         
           $search['source_ip'] = getVar('source_ip', NULL, $reqdata, 'string');
           $search['destination_ip'] = getVar('destination_ip', NULL, $reqdata, 'string');
           $limit = getVar('limit', 10, $reqdata, 'int');

           // UTILS    
           $utils['from_datetime'] = getVar('from_datetime', date("Y-m-d H:i:s", (time() - 900)), $reqdata, 'datetime');
           $utils['to_datetime'] = getVar('to_datetime', date("Y-m-d H:i:s"), $reqdata, 'datetime');

           $query = "SELECT * FROM ".$mydb->database_homer.".sip_capture WHERE (`date` BETWEEN '".$utils['from_datetime']."' AND '".$utils['to_datetime']."' )";

           $callwhere = array();
           
           if($search['method'] != NULL) $callwhere[] = "method = ".$mydb->quote($search['method']); 
           if($search['ruri_user'] != NULL) $callwhere[] = "ruri_user = ".$mydb->quote($search['ruri_user']); 
           if($search['from_user'] != NULL) $callwhere[] = "from_user = ".$mydb->quote($search['from_user']); 
           if($search['source_ip'] != NULL) $callwhere[] = "source_ip = ".$mydb->quote($search['source_ip']); 
           if($search['destination_ip'] != NULL) $callwhere[] = "destination_ip = ".$mydb->quote($search['destination_ip']); 
           
           if(count($callwhere)) $query .= " AND ( " .implode(" AND ", $callwhere). ")";

	   $order = " order by id DESC LIMIT ".$limit;

           $rows = $mydb->loadObjectList($query.$order);           
                      
           return $rows;
    }              


    
/********************************** STATISTIC *********************************************/

    function statisticMethod($param, $total) {

           date_default_timezone_set(HOMER_TIMEZONE);
           
           $mydb = $this->db;                            
           $skip_keys = array('location','max_records','from_datetime', 'to_datetime', 'unique','b2b','limit','node','logic_or');
           
           $reqdata = (array) $param;           
           $search = array();
	   $callwhere = array();

	   // SEARCH
           $search['method'] = getVar('method', NULL, $reqdata, 'string');           
           $search['cseq'] = getVar('cseq', NULL, $reqdata, 'string');
           $search['auth'] = getVar('auth', 0, $reqdata, 'int');
           $search['totag'] = getVar('totag', 0, $reqdata, 'int');
                  
           // UTILS    
           $utils['from_datetime'] = getVar('from_datetime', date("Y-m-d H:i:s", (time() - 1600)), $reqdata, 'datetime');
           $utils['to_datetime'] = getVar('to_datetime', date("Y-m-d H:i:s"), $reqdata, 'datetime');
           $utils['logic_or'] = getVar('logic_or', 0, $reqdata, 'int');
                      
           $and_or = $utils['logic_or'] ? " OR " : " AND ";

           if($total == 0) $fields = "id, from_date, to_date, method, auth, cseq, totag, total";
           else $fields = "id, from_date, to_date, method, auth, cseq, COUNT(id) as cnt, SUM(total) as total";

           $query = "SELECT ".$fields." FROM ".$mydb->database_homer.".stats_method WHERE (`to_date` BETWEEN '".$utils['from_datetime']."' AND '".$utils['to_datetime']."' )";

           $callwhere = array();
           
           if($search['method'] != NULL) $callwhere[] = "method = ".$mydb->quote($search['method']); 
           if($search['cseq'] != NULL) $callwhere[] = "cseq = ".$mydb->quote($search['cseq']); 
           if($search['auth'] != NULL && $search['auth'] != 0) $callwhere[] = "auth = ".$search['country']; 
           if($search['totag'] != NULL && $search['totag'] != 0) $callwhere[] = "totag = ".$search['destination']; 
           
           if(count($callwhere)) $query .= " AND ( " .implode(" AND ", $callwhere). ")";

           $order = "";
           
           if($total == 1) $order .= " GROUP BY method, cseq, auth, totag";                   	                         
	   $order .= " order by id DESC";

           $rows = $mydb->loadObjectList($query.$order);           

           return $rows;
    }              


    function statisticData($param, $total) {

           date_default_timezone_set(HOMER_TIMEZONE);
           
           $mydb = $this->db;                            
           $skip_keys = array('location','max_records','from_datetime', 'to_datetime', 'unique','b2b','limit','node','logic_or');
           
           $reqdata = (array) $param;           
           $search = array();
	   $callwhere = array();

	   // SEARCH
           $search['type'] = getVar('type', NULL, $reqdata, 'string');           

           // UTILS    
           $utils['from_datetime'] = getVar('from_datetime', date("Y-m-d H:i:s", (time() - 1600)), $reqdata, 'datetime');
           $utils['to_datetime'] = getVar('to_datetime', date("Y-m-d H:i:s"), $reqdata, 'datetime');
           $utils['logic_or'] = getVar('logic_or', 0, $reqdata, 'int');
                      
           $and_or = $utils['logic_or'] ? " OR " : " AND ";
                             
           /* total */
           if($total == 0) $fields = "id, from_date, to_date, type, total";
           else $fields = "id, from_date, to_date, type, COUNT(id) as cnt, SUM(total) as total";

           $query = "SELECT ".$fields." FROM ".$mydb->database_homer.".stats_data WHERE (`to_date` BETWEEN '".$utils['from_datetime']."' AND '".$utils['to_datetime']."' )";

           $callwhere = array();
           
           if($search['type'] != NULL) $callwhere[] = "type = ".$mydb->quote($search['type']); 
           
           if(count($callwhere)) $query .= " AND ( " .implode(" AND ", $callwhere). ")";
           
           $order = "";
         
           if($search['type'] == "asr" || $search['type'] == "ner") $order.=" AND total != 0";
           
           if($total == 1) $order .= " GROUP BY type";                   	   
	   $order .= " order by id DESC";

           $rows = $mydb->loadObjectList($query.$order);           
                      
           if($total == 1) {           
               foreach($rows as $row) {           
                    $row->result = $row->total;
                    if($row->type == "ner" || $row->type == "asr") {
                           if($row->cnt > 0) $row->result =  sprintf("%.2f", $row->total / $row->cnt);
                           else $row->result = 0;
                     }                                                     
               }                            
           }
                      
           return $rows;
    }              

    function statisticIP($param, $total) {

           date_default_timezone_set(HOMER_TIMEZONE);
           
           $mydb = $this->db;                            

           $reqdata = (array) $param;           
           $search = array();
	   $callwhere = array();

	   // SEARCH
           $search['source_ip'] = getVar('source_ip', NULL, $reqdata, 'string');           
           $search['method'] = getVar('method', NULL, $reqdata, 'string');           

           // UTILS    
           $utils['from_datetime'] = getVar('from_datetime', date("Y-m-d H:i:s", (time() - 1600)), $reqdata, 'datetime');
           $utils['to_datetime'] = getVar('to_datetime', date("Y-m-d H:i:s"), $reqdata, 'datetime');
           $utils['logic_or'] = getVar('logic_or', 0, $reqdata, 'int');
                      
           $and_or = $utils['logic_or'] ? " OR " : " AND ";
                             
           /* total */
           if($total == 0) $fields = "id, from_date, to_date, source_ip, method, total";
           else $fields = "id, from_date, to_date, source_ip, method, COUNT(id) as cnt, SUM(total) as total";

           $query = "SELECT ".$fields." FROM ".$mydb->database_homer.".stats_ip WHERE (`to_date` BETWEEN '".$utils['from_datetime']."' AND '".$utils['to_datetime']."' )";

           $callwhere = array();
           
           if($search['method'] != NULL) $callwhere[] = "method = ".$mydb->quote($search['method']); 
           if($search['source_ip'] != NULL) $callwhere[] = "source_ip = ".$mydb->quote($search['source_ip']); 
           
           if(count($callwhere)) $query .= " AND ( " .implode(" AND ", $callwhere). ")";
           
           $order = "";

           if($total == 1) $order .= " GROUP BY source_ip ORDER by total DESC";                   	   
	   else $order .= " order by total DESC";

           $rows = $mydb->loadObjectList($query.$order);           
                      
           return $rows;
    }              



    function statisticUserAgent($param, $total) {

           date_default_timezone_set(HOMER_TIMEZONE);
           
           $mydb = $this->db;                            
           $skip_keys = array('location','max_records','from_datetime', 'to_datetime', 'unique','b2b','limit','node','logic_or');
           
           $reqdata = (array) $param;           
           $search = array();
	   $callwhere = array();

	   // SEARCH
           $search['useragent'] = getVar('useragent', NULL, $reqdata, 'string');           
           $search['method'] = getVar('method', NULL, $reqdata, 'string');           

           // UTILS    
           $utils['from_datetime'] = getVar('from_datetime', date("Y-m-d H:i:s", (time() - 1600)), $reqdata, 'datetime');
           $utils['to_datetime'] = getVar('to_datetime', date("Y-m-d H:i:s"), $reqdata, 'datetime');
           $utils['logic_or'] = getVar('logic_or', 0, $reqdata, 'int');
                      
           $and_or = $utils['logic_or'] ? " OR " : " AND ";
                             
           /* total */
           if($total == 0) $fields = "id, from_date, to_date, useragent, method, total";
           else $fields = "id, from_date, to_date, useragent, method, COUNT(id) as cnt, SUM(total) as total";

           $query = "SELECT ".$fields." FROM ".$mydb->database_homer.".stats_useragent WHERE (`to_date` BETWEEN '".$utils['from_datetime']."' AND '".$utils['to_datetime']."' )";

           $callwhere = array();
           
           if($search['useragent'] != NULL) $callwhere[] = "useragent = ".$mydb->quote($search['useragent']); 
           if($search['method'] != NULL) $callwhere[] = "method = ".$mydb->quote($search['method']); 
           
           if(count($callwhere)) $query .= " AND ( " .implode(" AND ", $callwhere). ")";

           if($total == 1) $order .= " GROUP BY useragent, method ORDER by total DESC";                   	   
	   else $order .= " order by id DESC";

           $rows = $mydb->loadObjectList($query.$order);           
                      
           return $rows;
    }              
    
    function alarmData($param, $total) {

           date_default_timezone_set(HOMER_TIMEZONE);
           
           $mydb = $this->db;                            

           $reqdata = (array) $param;           
           $search = array();
	   $callwhere = array();

	   // SEARCH
           $search['type'] = getVar('type', NULL, $reqdata, 'string');           
           $search['status'] = getVar('status', 2, $reqdata, 'int');           

           $mintime = 864000;
           
           if($total == 2) $search['status']  = 1;

           // UTILS               
           $utils['from_datetime'] = getVar('from_datetime', date("Y-m-d H:i:s", (time() - $mintime)), $reqdata, 'datetime');
           $utils['to_datetime'] = getVar('to_datetime', date("Y-m-d H:i:s"), $reqdata, 'datetime');
           $utils['logic_or'] = getVar('logic_or', 0, $reqdata, 'int');
                      
           $and_or = $utils['logic_or'] ? " OR " : " AND ";
                             
           /* total */
           if($total == 0 || $total == 2) $fields = "id, create_date, type, total, source_ip, status, description";
           else $fields = "id, create_date, type, status, source_ip, COUNT(id) as cnt, SUM(total) as total";

           $query = "SELECT ".$fields." FROM ".$mydb->database_homer.".alarm_data WHERE (`create_date` BETWEEN '".$utils['from_datetime']."' AND '".$utils['to_datetime']."' )";

           $callwhere = array();
           
           if($search['type'] != NULL) $callwhere[] = "type = ".$mydb->quote($search['type']); 
           if($search['status'] != 2) $callwhere[] = "status = ".$mydb->quote($search['status']); 
           
           if(count($callwhere)) $query .= " AND ( " .implode(" AND ", $callwhere). ")";

           if($total == 1) $order .= " GROUP BY type";                   	             
	   $order .= " order by id DESC";

	   if($total == 2) $order .= " LIMIT 10";                   	             

           $rows = $mydb->loadObjectList($query.$order);           
                      
           return $rows;
    }              

    function replaceAlarm($param) {

           date_default_timezone_set(HOMER_TIMEZONE);
           
           $mydb = $this->db;                            
           
           $reqdata = (array) $param;           
           $search = array();
	   $callwhere = array();

	   // SEARCH
           $status = getVar('status', NULL, $reqdata, 'int');
           $id = getVar('id', NULL, $reqdata, 'int');
                  
           $query = "UPDATE ".$mydb->database_homer.".alarm_data SET status = ".$status." WHERE id=".$id;           

           $mydb->executeQuery($query);           
           return 1;
    } 

    function deleteAlarm($param) {

           date_default_timezone_set(HOMER_TIMEZONE);
           
           $mydb = $this->db;                                       
           $reqdata = (array) $param;           

	   // SEARCH
           $id = getVar('id', NULL, $reqdata, 'int');                  
           $query = "DELETE FROM ".$mydb->database_homer.".alarm_data WHERE id=".$id;           
           $mydb->executeQuery($query);           
           return 1;
    } 
    
    /* Function commented because there is already a global function
     * which has been fixed and this one doesn't appear to be used.
     * Travis Hegner
    function getVar($name, $default, $request, $type) {

        $val = isset($request[$name]) ? $request[$name] : $default;
 
        $type = strtoupper($type);

        #INT
        if(strcmp($type,"int") == 0) intval($val);
        #Float
        if(strcmp($type,"float") == 0) floatval($val);
        #String
        else if(strcmp($type,"string") == 0) {
                #Strip slashes
                if(get_magic_quotes_gpc()) $val = stripslashes($val);        
                return strval($val);
        }
        #Datetime
        else if(strcmp($type,"datetime") == 0) return date("Y-m-d H:i:s", strtotime($val));
        #Date
        else if(strcmp($type,"date") == 0) return date("Y-m-d H:i:s", strtotime($val));
        #Time
        else if(strcmp($type,"time") == 0) return date("H:i:s", strtotime($val));
        #Array
        else return $val;
    }
    */
       
}
