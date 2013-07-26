<?php
/*
 * HOMER Web Interface
 * Homer's index.php
 *
 * Copyright (C) 2011-2013 Alexandr Dubovikov <alexandr.dubovikov@gmail.com>
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


function processRequest() {
		if(isset($_SERVER['REQUEST_METHOD'])) $request_method = strtolower($_SERVER['REQUEST_METHOD']);
		else $request_method = "get";
		
		$return_obj		= new RestRequest();
		$data			= array();

		switch ($request_method)
		{
			// gets are easy...
			case 'get':
				$data = $_GET;
                                if(isset($_SERVER['REQUEST_METHOD'])) $return_obj->setRURI($_SERVER['REQUEST_URI']);
				break;			
			case 'post':			
				$data = $_POST;
				if(isset($_SERVER['REQUEST_METHOD'])) $return_obj->setRURI($_SERVER['REQUEST_URI']);
				break;
			case 'delete':			
				$data = $_GET;
				if(isset($_SERVER['REQUEST_METHOD'])) $return_obj->setRURI($_SERVER['REQUEST_URI']);
				break;
				// here's the tricky bit...				
			case 'put':
				parse_str(file_get_contents('php://input'), $put_vars);
				$data = $put_vars;
				break;
		}

		// store the method
		$return_obj->setMethod($request_method);		
		// set the raw data, so we can access it if needed (there may be
		// other pieces to your requests)
		$return_obj->setRequestVars($data);

		if(isset($data['data']))
		{
			// translate the JSON to an Object for use however you want
			$return_obj->setData(json_decode($data['data']));
		}
		return $return_obj;
}


function sendResponse($status = 200, $body = '', $content_type = 'text/html')
{
		header('HTTP/1.1 ' . $status . ' ' . RestUtils::getStatusCodeMessage($status));
		header('Content-type: ' . $content_type);
                header('Cache-Control: no-cache, must-revalidate');
                header('Access-Control-Allow-Origin: http://www.sipcapture.org/');
                header('Access-Control-Max-Age: 3600');                                         
                header('Server: Homer REST/1.0');                                         
                header('Expires: Mon, 26 Jul 1997 05:00:00 GMT');
                header('Access-Control-Allow-Methods: GET, POST, PUT, DELETE');

                echo $body;
                exit;		
}

function sendFile($status = 200, $filename, $filesize, $body)
{
		header('HTTP/1.1 ' . $status . ' ' . RestUtils::getStatusCodeMessage($status));
		header('Content-type: application/octet-stream');
		header('Content-Disposition: filename="'.$filename.'"');
		header('Content-length: '.$filesize);		        
                header('Cache-Control: no-cache, must-revalidate');
                header('Access-Control-Allow-Origin: http://www.sipcapture.org/');
                header('Access-Control-Max-Age: 3600');                                         
                header('Server: Homer REST/1.0');                                         
                header('Expires: Mon, 26 Jul 1997 05:00:00 GMT');
                header('Access-Control-Allow-Methods: GET, POST, PUT, DELETE');

                if(ob_get_length()) ob_clean();
                
                echo $body;
                
                exit;		
}

?>
