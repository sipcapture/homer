<?php
/*
 * HOMER Web Interface
 * Homer's alarm class
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


class HomerAlarm {

	//DB for internal save
        var $db;

        function setDB($db) {
                $this->db =  $db;
        }
      
 
        function sendMail($subject, $body, $html=1, $from=NULL, $to=NULL) {

		if(!preg_match("/^[^@]*@[^@]*\.[^@]*$/", $from)) $from = ALARM_FROMEMAIL;
		if(!preg_match("/^[^@]*@[^@]*\.[^@]*$/", $to)) $to = ALARM_TOEMAIL;

		$headers = "MIME-Version: 1.0\r\n"
		         ."Content-Transfer-Encoding: 8bit\r\n"
		         ."X-Priority: 1\r\n"
		         ."X-MSMail-Priority: High\r\n"
                         ."From: $from\r\n"
			 ."Reply-To: $from\r\n"
			 ."X-Mailer: PHP/" . phpversion() . "\r\n";
			 
                if($html == 1)$headers .= "Content-type: text/html; charset=iso-8859-2\r\n";
                else $headers.="Content-type: text/plain; charset=iso-8859-2\r\n";

		$success = @mail ($to, $subject, $body, $headers);
		return $success;
                
        }
        
        function sendSMS($email, $body) {

                $body = substr($body, 0, 140);                        
                return $this->sendMail("", $body, 0, NULL, $email);                
        }
                
}