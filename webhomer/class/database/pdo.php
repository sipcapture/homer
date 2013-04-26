<?php
/*
 * HOMER Web Interface
 * Homer's pdo.php
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


class HomerDB {
	//database setup 

       //MAKE SURE TO FILL IN DATABASE INFO
	var $hostname_logon = HOST;		//Database server LOCATION
	var $port_logon = PORT;			//Database PORT default MYSQL
	var $database_logon = DB;		//Database NAME
	var $username_logon = USER;		//Database USERNAME
	var $password_logon = PW;		//Database PASSWORD

	var $hostname_homer = HOMER_HOST;	//Database server LOCATION
	var $port_homer = HOMER_PORT ;		//Database server PORT. default MYSQL
	var $database_homer = HOMER_DB;		//Database NAME
	var $username_homer = HOMER_USER;	//Database USERNAME
	var $password_homer = HOMER_PW;		//Database PASSWORD
	
	//table fields
	var $user_table = 'homer_logon';	//Users table name
	var $user_column = 'useremail';		//USERNAME column (value MUST be valid email)
	var $pass_column = 'password';		//PASSWORD column
	var $user_level = 'userlevel';		//(optional) userlevel column
	
	//encryption
	var $encrypt = true;		//set to true to use md5 encryption for the password
	/* CONNECT */
	protected $connection;		//Our connection
  protected $resultCount;   //number of rows affected by a query. 

	//connect to database
	function dbconnect(){
		
		try {		      
          $dbstring = DATABASE.":host=".$this->hostname_logon. (($this->port_logon) ? ";port=".$this->port_logon : "" ) . ";dbname=".$this->database_logon;
		      $this->connection = new PDO($dbstring, $this->username_logon, $this->password_logon);
		} catch (PDOException $e){
		      die($e->getMessage());
		}
		return;
	}

	//connect to database
	function dbconnect_homer($node){
	
		$host = isset ($node->host) ? $node->host : NULL;
		$dbname = isset ($node->dbname) ? $node->dbname : NULL;
		$dbusername = isset ($node->dbusername) ? $node->dbusername : NULL;
		$dbpassword = isset ($node->dbpassword) ? $node->dbpassword : NULL;
		$dbport = isset ($node->dbport) ? $node->dbport : NULL;
		
		if(!$host) $host = $this->hostname_homer;
	
		try {
			$dbstring = DATABASE.":host=".$host.(($dbport) ? ";port=".$dbport : "" ).";dbname=".$dbname;
			$this->connection = new PDO($dbstring, $dbusername, $dbpassword);
		} catch (PDOException $e){
			try {
				// if connection is not establish, use HOMER_HOSTNAME from configuration.php
				$host = $this->hostname_homer;
				$dbstring = DATABASE.":host=".$host.(($this->port_logon) ? ";port=".$this->port_logon : "" ).";dbname=".$this->database_homer;
				$this->connection = new PDO($dbstring, $this->username_homer, $this->password_homer);
			} catch (PDOException $e){
				die($e->getMessage());
			}
		}
	
		return true;
	}
		
	//prevent injection
	function qry($query) {
	      $this->dbconnect();
        $args  = func_get_args();
        $query = array_shift($args);
        $query = str_replace("?", "%s", $query);        
        if(DATABASE == 'pgsql') $query = $this->toPgSql($query);
	if (property_exists($this->connection, 'quote')) $args  = array_map($this->connection->quote, $args);
        array_unshift($args,$query);
        $query = call_user_func_array('sprintf',$args);              
        $statement = $this->connection->prepare($query);
        $statement->execute(); 	                 
        $this->resultCount = $statement->rowCount();    
        $result = $statement->fetch();              
        if($result){
                return $result;
	      }else{
	              $error = "Error";
	              return $result;
        }
  }
        
        //prevent injection
	function makeQuery($query) {
	      $this->dbconnect();
        $args  = func_get_args();
        $query = array_shift($args);        
        $query = str_replace("?", "%s", $query);        
        if(DATABASE == 'pgsql') $query = $this->toPgSql($query);        
        if (property_exists($this->connection, 'quote')) $args  = array_map($this->connection->quote, $args);
        // Prevent sql injection. Thank Kai Oliver Quambusch for bug report.
        //$args = $this->custom_sql_escape($args);
        array_unshift($args,$query);        
        $query = call_user_func_array('sprintf',$args);
        return $query;
  }
	
  function getNodes (){
  	 
  	$this->dbconnect();
    //$query = "SELECT id,host,dbname,dbtables,dbusername,dbpassword,name FROM homer_nodes WHERE status = 1";
    $query = "SELECT * FROM homer_nodes WHERE status = 1";
  	return $this->loadObjectList($query);
  	 
  }
  
  function getAliases(){
  	//conect to DB
  	$this->dbconnect();
  	$query = "SELECT id,host,name FROM homer_hosts WHERE status = 1";
  	return $this->loadObjectList($query);
  
  }

	function executeQuery($query) {			
		//$result = mysql_query($query);
		if(DATABASE == 'pgsql') $query = $this->toPgSql($query);
		$statement = $this->connection->prepare($query);
    $result = $statement->execute();
  	$this->resultCount = $statement->rowCount();

		if(!$result) return false;
		else return true;
	}

	function loadObjectList($query) {

            if(DATABASE == 'pgsql') $query = $this->toPgSql($query);
            $statement = $this->connection->prepare($query);
            $statement->execute();		                	  
            $this->resultCount = $statement->rowCount();      
	    $result = $statement->fetchAll(PDO::FETCH_CLASS);
	    return $result;	        
	}
	
	function loadObjectArray($query) {
	
            if(DATABASE == 'pgsql') $query = $this->toPgSql($query);
            $statement = $this->connection->prepare($query);
            $statement->execute();		                	  
            $this->resultCount = $statement->rowCount();      
            $result = $statement->fetchAll();
    	    return $result;
	}
   
  function toPgSql($query) {
		$query = str_replace("`", '"', $query);
		return preg_replace('/[Ll][Ii][Mm][Ii][Tt]\s+([0-9]+)\s*,\s*([0-9]+)/', ' LIMIT ${2} OFFSET ${1}', $query);
	}

	function loadResult($query)
	{
          if(DATABASE == 'pgsql') $query = $this->toPgSql($query);
        	$statement = $this->connection->prepare($query);
        	$statement->execute();     
          $this->resultCount = $statement->rowCount();           
        	$result = $statement->fetch(PDO::FETCH_NUM);
        	return $result[0];        	                                             	
	}		
   
  function getResultCount(){
		return $this->resultCount;
  }

  function quote($val) {
         $this->dbconnect();
         return $this->connection->quote($val);
  }
                                    
  function custom_sql_escape($inp) {
    if(is_array($inp))
        return array_map(__METHOD__, $inp);
 
    if(!empty($inp) && is_string($inp)) {
        return str_replace(array('\\', "\0", "\n", "\r", "'", '"', "\x1a"), array('\\\\', '\\0', '\\n', '\\r', "\\'", '\\"', '\\Z'), $inp);
    }
 
    return $inp;
  }
  
}

?>
