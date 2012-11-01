<?php
/*
 * HOMER Web Interface
 * Homer's internal auth
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

require("class/auth/index.php");  
require("class/auth/internal/settings.php");  

class HomerAuthentication extends Authentication {
	
	var $encrypt = true;
	var $pass_column = "password";
	var $user_column = "useremail";
        var $user_table = "homer_logon";
        var $user_level = "userlevel";                             
                        
	//login function
	function login($username, $password){

		if($this->encrypt == true)$password = md5($password);	

		$mydb = $this->db;
		$query  = $mydb->makeQuery("SELECT * FROM ".$this->user_table." WHERE ".$this->user_column."='?' AND ".$this->pass_column." = '?';" , $username, $password);
		$rows = $mydb->loadObjectList($query);
		if(count($rows) > 0) {		
			$row = $rows[0];
			$u=$this->user_column;
			$p=$this->pass_column;		
			$l=$this->user_level;							
			if($row->$u !="" && $row->$p !=""){
				$_SESSION['loggedin'] = $row->$u;
				$_SESSION['userlevel'] = $row->$l;
				return true;	
			}else{
				session_destroy();
				return false;
			}
		}else{
			return false;
		}		
	}
	
	//reset password
	function passwordReset($username, $user_table, $pass_column, $user_column){

		//generate new password
		$newpassword = $this->createPassword();
		
		//update database with new password
		$query = "UPDATE ".$this->user_table." SET ".$this->pass_column."='".$newpassword."' WHERE ".$this->user_column."='".stripslashes($username)."'";
		if(!$db->executeQuery($query)) {
			die("No update possible");		
			exit;		
		}
		
		$to = stripslashes($username);
		//some injection protection
		$illigals=array("n", "r","%0A","%0D","%0a","%0d","bcc:","Content-Type","BCC:","Bcc:","Cc:","CC:","TO:","To:","cc:","to:");
		$to = str_replace($illigals, "", $to);
		$getemail = explode("@",$to);
		
		//send only if there is one email
		if(sizeof($getemail) > 2){
			return false;	
		}else{
			//send email
			$from = $_SERVER['SERVER_NAME'];
			$subject = "Password Reset: ".$_SERVER['SERVER_NAME'];
			$msg = "<p>Your new password is: ".$newpassword."</p>";
			
			//now we need to set mail headers
			$headers = "MIME-Version: 1.0 rn" ;
			$headers .= "Content-Type: text/html; rn" ;
			$headers .= "From: $from  rn" ;
			
			//now we are ready to send mail
			$sent = mail($to, $subject, $msg, $headers);
			if($sent){
				return true; 
			}else{
				return false;	
			}
		}
	}
	
	//create random password with 8 alphanumerical characters
	function createPassword() {
		$chars = "abcdefghijkmnopqrstuvwxyz023456789";
		srand((double)microtime()*1000000);
		$i = 0;
		$pass = '' ;
		while ($i <= 7) {
			$num = rand() % 33;
			$tmp = substr($chars, $num, 1);
			$pass = $pass . $tmp;
			$i++;
		}
		return $pass;
	}
}

?>