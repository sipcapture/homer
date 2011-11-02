<?php
/**
 * (c) 2007-2009 Chris Maciejewski
 * 
 * Permission is hereby granted, free of charge, to any person obtaining 
 * a copy of this software and associated documentation files 
 * (the "Software"), to deal in the Software without restriction, 
 * including without limitation the rights to use, copy, modify, merge,
 * publish, distribute, sublicense, and/or sell copies of the Software, 
 * and to permit persons to whom the Software is furnished to do so, 
 * subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included 
 * in all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS 
 * OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, 
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL 
 * THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER 
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING 
 * FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER 
 * DEALINGS IN THE SOFTWARE.
 * 
 */

/**
 * PHP SIP UAC class
 * 
 * @ingroup  API
 * @author Chris Maciejewski <chris@wima.co.uk>
 * 
 * @version    SVN: $Id: PhpSIP.class.php 24 2009-11-25 11:39:57Z level7systems $
 */
require_once 'PhpSIP.Exception.php';

class PhpSIP
{
  private $debug = false;
  
  /**
   * Min port
   */
  private $min_port = 5065;
  
  /**
   * Max port
   */
  private $max_port = 5265;
  
  /**
   * Final Response timer (in seconds)
   */
  private $fr_timer = 7;
  
  /**
   * Lock file
   */
  private $lock_file;
  
  /**
   * Allowed methods array
   */
  private $allowed_methods = array(
    "CANCEL","NOTIFY", "INVITE","BYE","REFER","OPTIONS","SUBSCRIBE","MESSAGE"
  );
  
  /**
   * Dialog established
   */
  private $dialog = false;
  
  /**
   * The opened socket we listen for incoming SIP messages
   */
  private $socket;
  
  /**
   * Source IP address
   */
  private $src_ip;
  
  /**
   * Source IP address
   */
  private $user_agent = 'PHP SIP';
  
  /**
   * CSeq
   */
  private $cseq = 20;
  
  /**
   * Source port
   */
  private $src_port;
  
  /**
   * Call ID
   */
  private $call_id;
  
  /**
   * Contact
   */
  private $contact;
  
  /**
   * Request URI
   */
  private $uri;
  
  /**
   * Request host
   */
  private $host;
  
  /**
   * Request port
   */
  private $port = 5060;
  
  /**
   * Outboud SIP proxy
   */
  private $proxy;
  
  /**
   * Method
   */
  private $method;
  
  /**
   * Auth username
   */
  private $username;
  
  /**
   * Auth password
   */
  private $password;
  
  /**
   * To
   */
  private $to;
  
  /**
   * To tag
   */
  private $to_tag;
  
  /**
   * From
   */
  private $from;
  
  /**
   * From User
   */
  private $from_user;

  /**
   * From tag
   */
  private $from_tag;
  
  /**
   * Via tag
   */
  private $via;
  
  /**
   * Content type
   */
  private $content_type;
  
  /**
   * Body
   */
  private $body;
  
  /**
   * Received Response
   */
  private $response; // whole response body
  private $res_code;
  private $res_contact;
  private $res_cseq_method;
  private $res_cseq_number;

  /**
   * Received Request
   */
  private $req_method;
  private $req_cseq_method;
  private $req_cseq_number;
  private $req_contact;
  
  /**
   * Authentication
   */
  private $auth;
  
  /**
   * Routes
   */
  private $routes = array();
  
  /**
   * Request vias
   */
  private $request_via = array();
  
  /**
   * Additional headers
   */
  private $extra_headers = array();
  
  /**
   * Constructor
   * 
   * @param $src_ip Ip address to bind (optional)
   */
  public function __construct($src_ip = null)
  {
    if (!function_exists('socket_create'))
    {
      throw new PhpSIPException("socket_create() function missing.");
    }
    
    if (!$src_ip)
    {
      // running in a web server
      if (isset($_SERVER['SERVER_ADDR']))
      {
        $src_ip = $_SERVER['SERVER_ADDR'];
      }
      // running from command line
      else
      {
        $addr = gethostbynamel(php_uname('n'));
        
        if (!is_array($addr) || !isset($addr[0]) || substr($addr[0],0,3) == '127')
        {
          throw new PhpSIPException("Failed to obtain IP address to bind. Please set bind address manualy.");
        }
      
        $src_ip = $addr[0];
      }
    }
    
    $this->src_ip = $src_ip;
    
    $this->lock_file = rtrim(sys_get_temp_dir(),DIRECTORY_SEPARATOR).DIRECTORY_SEPARATOR.'phpSIP.lock';
    
    $this->createSocket();
  }
  
  /**
   * Destructor
   */
  public function __destruct()
  {
    $this->closeSocket();
  }
  
  /**
   * Sets debuggin ON/OFF
   * 
   * @param bool $status
   */
  public function setDebug($status = false)
  {
    $this->debug = $status;
  }
  
  /**
   * Gets src IP
   * 
   * @return string
   */
  public function getSrcIp()
  {
    return $this->src_ip;
  }
  
  /**
   * Gets port number to bind
   */
  private function getPort()
  {
    if ($this->min_port > $this->max_port)
    {
      throw new PhpSIPException ("Min port is bigger than max port.");
    }
    
    // waiting until file will be locked for writing 
    // (1000 milliseconds as timeout)
    $fp = @fopen($this->lock_file, 'a+b');
    
    if (!$fp)
    {
      throw new PhpSIPException ("Failed to open lock file ".$this->lock_file);
    }
    
    $startTime = microtime();
    
    do
    {
      $canWrite = flock($fp, LOCK_EX);
      // If lock not obtained sleep for 0 - 100 milliseconds,
      // to avoid collision and CPU load
      if(!$canWrite) usleep(round(rand(0, 100)*1000));
      
    } while ((!$canWrite)and((microtime()-$startTime) < 1000));
    
    if (!$canWrite)
    {
      throw new PhpSIPException ("Failed to lock a file in 1000 ms.");
    }
    
    //file was locked
    $size = filesize($this->lock_file);
    if ($size)
    {
      $contents = fread($fp, $size);
      $pids = explode(",",$contents);
    }
    else
    {
      $pids = false;
    }
    
    ftruncate($fp, 0);
    
    // we are the first one to run, initialize "PID" => "port number" array
    if (!$pids)
    {
      if (!fwrite($fp, $this->min_port))
      {
        throw new PhpSIPException("Fail to write data to a lock file.");
      }
      
      $this->src_port =  $this->min_port;
    }
    // there are other programs running now
    else
    {
      // check if there are any empty ports left
      if (count($pids) >= ($this->max_port - $this->min_port))
      {
        throw new PhpSIPException("No more ports left to bind.");
      }
      
      asort($pids,SORT_NUMERIC);
      
      $prev = current($pids);
      
      if ($prev > $this->min_port)
      {
        $src_port = $this->min_port;
      }
      else
      {
        foreach ($pids as $port)
        {
          if (($port - $prev) > 1)
          {
            $src_port = $prev + 1;
            break;
          }
          
          $prev = $port;
        }
        
        if (($prev + 1) >= $this->max_port)
        {
          throw new PhpSIPException("No more ports left to bind. We shouldn't be here!");
        }
        
        $src_port = $prev + 1;
      }
      
      if (in_array($src_port,$pids))
      {
        throw new PhpSIPException("Fail to obtain free port number.");
      }
      
      $pids[] = $src_port;
      
      if (!fwrite($fp, implode(",",$pids)))
      {
        throw new PhpSIPException("Failed to write data to lock file.");
      }
      
      $this->src_port = $src_port;
    }
    
    if (!fclose($fp))
    {
      throw new PhpSIPException("Failed to close lock_file");
    }
    
  }
  
  /**
   * Releases port
   */
  private function releasePort()
  {
    // waiting until file will be locked for writing 
    // (1000 milliseconds as timeout)
    $fp = fopen($this->lock_file, 'r+b');
    
    if (!$fp)
    {
      throw new PhpSIPException("Can't open lock file.");
    }
    
    $startTime = microtime();
    
    do
    {
      $canWrite = flock($fp, LOCK_EX);
      // If lock not obtained sleep for 0 - 100 milliseconds,
      // to avoid collision and CPU load
      if(!$canWrite) usleep(round(rand(0, 100)*1000));
      
    } while ((!$canWrite)and((microtime()-$startTime) < 1000));
    
    if (!$canWrite)
    {
      throw new PhpSIPException("Failed to lock a file in 1000 ms.");
    }
    
    clearstatcache();
    
    $size = filesize($this->lock_file);
    $content = fread($fp,$size);
    
    //file was locked
    $pids = explode(",",$content);
    
    $key = array_search($this->src_port,$pids);
    
    unset($pids[$key]);
    
    if (count($pids) === 0)
    {
      if (!fclose($fp))
      {
        throw new PhpSIPException("Failed to close lock_file");
      }
      
      if (!unlink($this->lock_file))
      {
        throw new PhpSIPException("Failed to delete lock_file.");
      }
    }
    else
    {
      ftruncate($fp, 0);
      
      if (!fwrite($fp, implode(",",$pids)))
      {
        throw new PhpSIPException("Failed to save data in lock_file");
      }
      
      if (!fclose($fp))
      {
        throw new PhpSIPException("Failed to close lock_file");
      }
    }
  }
  
  /**
   * Adds aditional header
   * 
   * @param string $header
   */
  public function addHeader($header)
  {
    $this->extra_headers[] = $header;
  }
  
  /**
   * Sets From header
   * 
   * @param string $from
   */
  public function setFrom($from)
  {
    if (preg_match('/<.*>$/',$from))
    {
      $this->from = $from;
    }
    else
    {
      $this->from = '<'.$from.'>';
    }
    
    $m = array();
    if (!preg_match('/sip:(.*)@/i',$this->from,$m))
    {
      throw new PhpSIPException('Failed to parse From username.');
    }
    
    $this->from_user = $m[1];
  }
  
  /**
   * Sets method
   * 
   * @param string $method
   */
  public function setMethod($method)
  {
    if (!in_array($method,$this->allowed_methods))
    {
      throw new PhpSIPException('Invalid method.');
    }
    
    $this->method = $method;
    
    if ($method == 'INVITE')
    {
      $body = "v=0\r\n";
      $body.= "o=click2dial 0 0 IN IP4 ".$this->src_ip."\r\n";
      $body.= "s=click2dial call\r\n";
      $body.= "c=IN IP4 ".$this->src_ip."\r\n";
      $body.= "t=0 0\r\n";
      $body.= "m=audio 8000 RTP/AVP 0 8 18 3 4 97 98\r\n";
      $body.= "a=rtpmap:0 PCMU/8000\r\n";
      $body.= "a=rtpmap:18 G729/8000\r\n";
      $body.= "a=rtpmap:97 ilbc/8000\r\n";
      $body.= "a=rtpmap:98 speex/8000\r\n";
      
      $this->body = $body;
      
      $this->setContentType(null);
    }
    
    if ($method == 'REFER')
    {
      $this->setBody('');
    }
    
    if ($method == 'CANCEL')
    {
      $this->setBody('');
      $this->setContentType(null);
    }
    
    if ($method == 'MESSAGE')
    {
      $this->setContentType(null);
    }
  }
  
  /**
   * Sets SIP Proxy
   * 
   * @param $proxy
   */
  public function setProxy($proxy)
  {
    $this->proxy = $proxy;
  }
  
  /**
   * Sets request URI
   *
   * @param string $uri
   */
  public function setUri($uri)
  {
    if (strpos($uri,'sip:') === false)
    {
      throw new PhpSIPException("Only sip: URI supported.");
    }
    
    $this->uri = $uri;
    $this->to = '<'.$uri.'>';
    
    if ($this->proxy)
    {
      if (strpos($this->proxy,':'))
      {
        $temp = explode(":",$this->proxy);
        
        $this->host = $temp[0];
        $this->port = $temp[1];
      }
      else
      {
        $this->host = $this->proxy;
      }
    }
    else
    {
      $url = str_replace("sip:","sip://",$uri);
      
      if (!$url = @parse_url($url))
      {
        throw new PhpSIPException("Failed to parse URI.");
      }
      
      $this->host = $url['host'];
      
      if (isset($url['port']))
      {
        $this->port = $url['port'];
      }
    }
  }
  
  /**
   * Sets username
   *
   * @param string $username
   */
  public function setUsername($username)
  {
    $this->username = $username;
  }
  
  /**
   * Sets User Agent
   *
   * @param string $user_agent
   */
  public function setUserAgent($user_agent)
  {
    $this->user_agent = $user_agent;
  }
  
  /**
   * Sets password
   *
   * @param string $password
   */
  public function setPassword($password)
  {
    $this->password = $password;
  }
  
  /**
   * Sends SIP request
   * 
   * @return string Reply 
   */
  public function send()
  {
    if (!$this->from)
    {
      throw new PhpSIPException('Missing From.');
    }
    
    if (!$this->method)
    {
      throw new PhpSIPException('Missing Method.');
    }
    
    if (!$this->uri)
    {
      throw new PhpSIPException('Missing URI.');
    }
    
    $data = $this->formatRequest();
    
    $this->sendData($data);
    
    $this->readResponse();
    
    if ($this->method == 'CANCEL' && $this->res_code == '200')
    {
      $i = 0;
      while (substr($this->res_code,0,1) != '4' && $i < 2)
      {
        $this->readResponse();
        $i++;
      }
    }
    
    if ($this->res_code == '407')
    {
      $this->cseq++;
      
      $this->auth();
      
      $data = $this->formatRequest();
      
      $this->sendData($data);
      
      $this->readResponse();
    }
    
    if ($this->res_code == '401')
    {
      $this->cseq++;
      
      $this->authWWW();
      
      $data = $this->formatRequest();
      
      $this->sendData($data);
      
      $this->readResponse();
    }
    
    if (substr($this->res_code,0,1) == '1')
    {
      $i = 0;
      while (substr($this->res_code,0,1) == '1' && $i < 4)
      {
        $this->readResponse();
        $i++;
      }
    }
    
    $this->extra_headers = array();
    $this->cseq++;
    
    return $this->res_code;
  }
  
  /**
   * Sends data
   */
  private function sendData($data)
  {
    usleep(10000);
    
    if (!@socket_sendto($this->socket, $data, strlen($data), 0, $this->host, $this->port))
    {
      $err_no = socket_last_error($this->socket);
      throw new PhpSIPException("Failed to send data. ".socket_strerror($err_no));
    }
    
    if ($this->debug)
    {
      $temp = explode("\r\n",$data);
      
      echo "--> ".$temp[0]."\n";
    }
  }
  
  /**
   * Listen for request
   * 
   * @todo This needs to be improved
   */
  public function listen($method)
  {
    $i = 0;
    while ($this->req_method != $method)
    {
      $this->readResponse(); 
      
      $i++;
      
      if ($i > 5)
      {
        throw new PhpSIPException("Unexpected request ".$this->req_method."received.");
      }
    }
  }
  
  /**
   * Reads response
   */
  private function readResponse()
  {
    $from = "";
    $port = 0;
    
    if (!@socket_recvfrom($this->socket, $this->response, 10000, 0, $from, $port))
    {
      $this->res_code = "No final response in fr_timer seconds.";
      return $this->res_code;
    }
    
    if ($this->debug)
    {
      $temp = explode("\r\n",$this->response);
      
      echo "<-- ".$temp[0]."\n";
    }
    
    // Response
    $result = array();
    if (preg_match('/^SIP\/2\.0 ([0-9]{3})/',$this->response,$result))
    {
      $this->res_code = trim($result[1]);
      
      $res_class = substr($this->res_code,0,1);
      if ($res_class == '1' || $res_class == '2')
      {
        $this->dialog = true;
      }
      
      $this->parseResponse();
    }
    // Request
    else
    {
      $this->parseRequest();
    }
  }
  
  /**
   * Parse Response
   */
  private function parseResponse()
  {
    // To tag
    $result = array();
    if (preg_match('/^To: .*;tag=(.*)$/im',$this->response,$result))
    {
      $this->to_tag = trim($result[1]);
    }
    
    // Route
    $result = array();
    if (preg_match_all('/^Record-Route: (.*)$/im',$this->response,$result))
    {
      foreach ($result[1] as $route)
      {
        if (!in_array(trim($route),$this->routes))
        {
          $this->routes[] = trim($route);
        }
      }
    }
    
    // Request via
    $result = array();
    $this->request_via = array();
    if (preg_match_all('/^Via: (.*)$/im',$this->response,$result))
    {
      foreach ($result[1] as $via)
      {
        $this->request_via[] = trim($via);
      }
    }
    
    // Response contact
    $result = array();
    if (preg_match('/^Contact:.*<(.*)>/im',$this->response,$result))
    {
      $this->res_contact = trim($result[1]);
      
      $semicolon = strpos($this->res_contact,";");
      
      if ($semicolon !== false)
      {
        $this->res_contact = substr($this->res_contact,0,$semicolon);
      }
    }
    
    // Response CSeq method
    $result = array();
    if (preg_match('/^CSeq: [0-9]+ (.*)$/im',$this->response,$result))
    {
      $this->res_cseq_method = trim($result[1]);
    }
    
    // ACK 2XX-6XX - only invites - RFC3261 17.1.2.1
    if ($this->res_cseq_method == 'INVITE' && in_array(substr($this->res_code,0,1),array('2','3','4','5','6')))
    {
      $this->ack();
    }
    
    return $this->res_code;
  }
  
  /**
   * Parse Request
   */
  private function parseRequest()
  {
    $temp = explode("\r\n",$this->response);
    $temp = explode(" ",$temp[0]);
    $this->req_method = trim($temp[0]);
    
    // Route
    $result = array();
    if (preg_match_all('/^Record-Route: (.*)$/im',$this->response,$result))
    {
      foreach ($result[1] as $route)
      {
        if (!in_array(trim($route),$this->routes))
        {
          $this->routes[] = trim($route);
        }
      }
    }
    
    // Request via
    $result = array();
    $this->request_via = array();
    if (preg_match_all('/^Via: (.*)$/im',$this->response,$result))
    {
      foreach ($result[1] as $via)
      {
        $this->request_via[] = trim($via);
      }
    }
    
    // Method contact
    $result = array();
    if (preg_match('/^Contact: <(.*)>/im',$this->response,$result))
    {
      $this->req_contact = trim($result[1]);
      
      $semicolon = strpos($this->res_contact,";");
      
      if ($semicolon !== false)
      {
        $this->res_contact = substr($this->res_contact,0,$semicolon);
      }
    }
    
    // Response CSeq method
    if (preg_match('/^CSeq: [0-9]+ (.*)$/im',$this->response,$result))
    {
      $this->req_cseq_method = trim($result[1]);
    }
    
    // Response CSeq number
    if (preg_match('/^CSeq: ([0-9]+) .*$/im',$this->response,$result))
    {
      $this->req_cseq_number = trim($result[1]);
    }
  }
  
  /**
   * Send Response
   * 
   * @param int $code     Response code
   * @param string $text  Response text
   */
  public function reply($code,$text)
  {
    $r = 'SIP/2.0 '.$code.' '.$text."\r\n";
    // Via
    foreach ($this->request_via as $via)
    {
      $r.= 'Via: '.$via."\r\n";
    }
    // From
    $r.= 'From: '.$this->from.';tag='.$this->to_tag."\r\n";
    // To
    $r.= 'To: '.$this->to.';tag='.$this->from_tag."\r\n";
    // Call-ID
    $r.= 'Call-ID: '.$this->call_id."\r\n";
    //CSeq
    $r.= 'CSeq: '.$this->req_cseq_number.' '.$this->req_cseq_method."\r\n";
    // Max-Forwards
    $r.= 'Max-Forwards: 70'."\r\n";
    // User-Agent
    $r.= 'User-Agent: '.$this->user_agent."\r\n";
    // Content-Length
    $r.= 'Content-Length: 0'."\r\n";
    $r.= "\r\n";
    
    $this->sendData($r);
  }
  
  /**
   * ACK
   */
  private function ack()
  {
    if ($this->res_cseq_method == 'INVITE' && $this->res_code == '200')
    {
      $a = 'ACK '.$this->res_contact.' SIP/2.0'."\r\n";
    }
    else
    {
      $a = 'ACK '.$this->uri.' SIP/2.0'."\r\n";
    }
    // Via
    $a.= 'Via: '.$this->via."\r\n";
    // Route
    if ($this->routes)
    {
      foreach ($this->routes as $route)
      {
        $a.= 'Route: '.$route."\r\n";
      }
    }
    // From
    if (!$this->from_tag) $this->setFromTag();
    $a.= 'From: '.$this->from.';tag='.$this->from_tag."\r\n";
    // To
    if ($this->to_tag)
      $a.= 'To: '.$this->to.';tag='.$this->to_tag."\r\n";
    else
      $a.= 'To: '.$this->to."\r\n";
    // Call-ID
    if (!$this->call_id) $this->setCallId();
    $a.= 'Call-ID: '.$this->call_id."\r\n";
    //CSeq
    $a.= 'CSeq: '.$this->cseq.' ACK'."\r\n";
    // Authentication
    if ($this->res_code == '200' && $this->auth)
    {
      $a.= 'Proxy-Authorization: '.$this->auth."\r\n";
    }
    // Max-Forwards
    $a.= 'Max-Forwards: 70'."\r\n";
    // User-Agent
    $a.= 'User-Agent: '.$this->user_agent."\r\n";
    // Content-Length
    $a.= 'Content-Length: 0'."\r\n";
    $a.= "\r\n";
    
    $this->sendData($a);
  }
  
  /**
   * Formats SIP request
   * 
   * @return string
   */
  private function formatRequest()
  {
    if (in_array($this->method,array('BYE','REFER','SUBSCRIBE')))
    {
      $r = $this->method.' '.$this->res_contact.' SIP/2.0'."\r\n";
    }
    else
    {
      $r = $this->method.' '.$this->uri.' SIP/2.0'."\r\n";
    }
    // Via
    if ($this->method != 'CANCEL')
    {
      $this->setVia();
    }
    $r.= 'Via: '.$this->via."\r\n";
    // Route
    if ($this->method != 'CANCEL' && $this->routes)
    {
      foreach ($this->routes as $route)
      {
        $r.= 'Route: '.$route."\r\n";
      }
    }
    // From
    if (!$this->from_tag) $this->setFromTag();
    $r.= 'From: '.$this->from.';tag='.$this->from_tag."\r\n";
    // To
    if (!in_array($this->method,array("INVITE","CANCEL","NOTIFY")) && $this->to_tag)
      $r.= 'To: '.$this->to.';tag='.$this->to_tag."\r\n";
    else
      $r.= 'To: '.$this->to."\r\n";
    // Authentication
    if ($this->auth)
    {
      $r.= $this->auth."\r\n";
      $this->auth = null;
    }
    // Call-ID
    if (!$this->call_id) $this->setCallId();
    $r.= 'Call-ID: '.$this->call_id."\r\n";
    //CSeq
    if ($this->method == 'CANCEL')
    {
      $this->cseq--;
    }
    $r.= 'CSeq: '.$this->cseq.' '.$this->method."\r\n";
    // Contact
    if ($this->method != 'MESSAGE')
    {
      $r.= 'Contact: <sip:'.$this->from_user.'@'.$this->src_ip.':'.$this->src_port.'>'."\r\n";
    }
    // Content-Type
    if ($this->content_type)
    {
      $r.= 'Content-Type: '.$this->content_type."\r\n";
    }
    // Max-Forwards
    $r.= 'Max-Forwards: 70'."\r\n";
    // User-Agent
    $r.= 'User-Agent: '.$this->user_agent."\r\n";
    // Additional header
    foreach ($this->extra_headers as $header)
    {
      $r.= $header."\r\n";
    }
    // Content-Length
    $r.= 'Content-Length: '.strlen($this->body)."\r\n";
    $r.= "\r\n";
    $r.= $this->body;
    
    return $r;
  }
  
  /**
   * Sets body
   */
  public function setBody($body)
  {
    $this->body = $body;
  }
  
  /**
   * Sets Content Type
   */
  public function setContentType($content_type = null)
  {
    if ($content_type !== null)
    {
      $this->content_type = $content_type;
    }
    else
    {
      switch ($this->method)
      {
        case 'INVITE':
          $this->content_type = 'application/sdp';
          break;
        case 'MESSAGE':
          $this->content_type = 'text/html; charset=utf-8';
          break;
        default:
          $this->content_type = null;
      }
    }
  }
  
  /**
   * Sets Via header
   */
  private function setVia()
  {
    $rand = rand(100000,999999);
    $this->via = 'SIP/2.0/UDP '.$this->src_ip.':'.$this->src_port.';rport;branch=z9hG4bK'.$rand;
  }
  
  /**
   * Sets from tag
   */
  private function setFromTag()
  { 
    $this->from_tag = rand(10000,99999);
  }
  
  /**
   * Sets call id
   */
  private function setCallId()
  {
    $this->call_id = md5(uniqid()).'@'.$this->src_ip;
  }
  
  /**
   * Gets value of the header from the previous request
   * 
   * @param string $name Header name
   * 
   * @return string or false
   */
  public function getHeader($name)
  {
    if (preg_match('/^'.$name.': (.*)$/m',$this->response,$result))
    {
      return trim($result[1]);
    }
    else
    {
      return false;
    }
  }
  
  /**
   * Calculates Digest authentication response
   * 
   */
  private function auth()
  {
    if (!$this->username)
    {
      throw new PhpSIPException("Missing username");
    }
    
    if (!$this->password)
    {
      throw new PhpSIPException("Missing password");
    }
    
    // realm
    $result = array();
    if (!preg_match('/^Proxy-Authenticate: .* realm="(.*)"/imU',$this->response, $result))
    {
      throw new PhpSIPException("Can't find realm in proxy-auth");
    }
    
    $realm = $result[1];
    
    // nonce
    $result = array();
    if (!preg_match('/^Proxy-Authenticate: .* nonce="(.*)"/imU',$this->response, $result))
    {
      throw new PhpSIPException("Can't find nonce in proxy-auth");
    }
    
    $nonce = $result[1];
    
    $ha1 = md5($this->username.':'.$realm.':'.$this->password);
    $ha2 = md5($this->method.':'.$this->uri);
    
    $res = md5($ha1.':'.$nonce.':'.$ha2);
    
    $this->auth = 'Proxy-Authorization: Digest username="'.$this->username.'", realm="'.$realm.'", nonce="'.$nonce.'", uri="'.$this->uri.'", response="'.$res.'", algorithm=MD5';
  }
  
  /**
   * Calculates WWW authorization response
   * 
   */
  private function authWWW()
  {
    if (!$this->username)
    {
      throw new PhpSIPException("Missing auth username");
    }
    
    if (!$this->password)
    {
      throw new PhpSIPException("Missing auth password");
    }
    
    $qop_present = false;
    if (strpos($this->response,'qop=') !== false)
    {
      $qop_present = true;
      
      // we can only do qop="auth"
      if  (strpos($this->response,'qop="auth"') === false)
      {
        throw new PhpSIPException('Only qop="auth" digest authentication supported.');
      }
    }
    
    // realm
    $result = array();
    if (!preg_match('/^WWW-Authenticate: .* realm="(.*)"/imU',$this->response, $result))
    {
      throw new PhpSIPException("Can't find realm in www-auth");
    }
    
    $realm = $result[1];
    
    // nonce
    $result = array();
    if (!preg_match('/^WWW-Authenticate: .* nonce="(.*)"/imU',$this->response, $result))
    {
      throw new PhpSIPException("Can't find nonce in www-auth");
    }
    
    $nonce = $result[1];
    
    $ha1 = md5($this->username.':'.$realm.':'.$this->password);
    $ha2 = md5($this->method.':'.$this->uri);
    
    if ($qop_present)
    {
      $cnonce = md5(time());
      
      $res = md5($ha1.':'.$nonce.':00000001:'.$cnonce.':auth:'.$ha2);
    }
    else
    {
      $res = md5($ha1.':'.$nonce.':'.$ha2);
    }
    
    $this->auth = 'Authorization: Digest username="'.$this->username.'", realm="'.$realm.'", nonce="'.$nonce.'", uri="'.$this->uri.'", response="'.$res.'", algorithm=MD5';
    
    if ($qop_present)
    {
      $this->auth.= ', qop="auth", nc="00000001", cnonce="'.$cnonce.'"';
    }
  }
  
  /**
   * Create network socket
   *
   * @return bool True on success
   */
  private function createSocket()
  { 
    $this->getPort();
    
    if (!$this->src_ip)
    {
      throw new PhpSIPException("Source IP not defined.");
    }
    
    if (!$this->socket = @socket_create(AF_INET, SOCK_DGRAM, SOL_UDP))
    {
      $err_no = socket_last_error($this->socket);
      throw new PhpSIPException (socket_strerror($err_no));
    }
    
    if (!@socket_bind($this->socket, $this->src_ip, $this->src_port))
    {
      $err_no = socket_last_error($this->socket);
      throw new PhpSIPException ("Failed to bind ".$this->src_ip.":".$this->src_port." ".socket_strerror($err_no));
    }
    
    if (!@socket_set_option($this->socket, SOL_SOCKET, SO_RCVTIMEO, array("sec"=>$this->fr_timer,"usec"=>0)))
    {
      $err_no = socket_last_error($this->socket);
      throw new PhpSIPException (socket_strerror($err_no));
    }
    
    if (!@socket_set_option($this->socket, SOL_SOCKET, SO_SNDTIMEO, array("sec"=>5,"usec"=>0)))
    {
      $err_no = socket_last_error($this->socket);
      throw new PhpSIPException (socket_strerror($err_no));
    }
  }
  
  /**
   * Close the connection
   *
   * @return bool True on success
   */
  private function closeSocket()
  {
    $this->releasePort();
    
    socket_close($this->socket);
  }
  
  /**
   * Resets callid, to/from tags etc.
   * 
   */
  public function newCall()
  {
    $this->cseq = 20;
    $this->call_id = null;
    $this->to_tag = null;;
    $this->from_tag = null;;
    
    /**
     * Body
     */
    $this->body = null;
    
    /**
     * Received Response
     */
    $this->response = null;
    $this->res_code = null;
    $this->res_contact = null;
    $this->res_cseq_method = null;
    $this->res_cseq_number = null;

    /**
     * Received Request
     */
    $this->req_method = null;
    $this->req_cseq_method = null;
    $this->req_cseq_number = null;
    $this->req_contact = null;
    
    $this->routes = array();
    $this->request_via = array();
  }
}

?>
