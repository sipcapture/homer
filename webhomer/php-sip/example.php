<?php
require_once('PhpSIP.class.php');

/* Sends test message */

$item = $_GET['proxy'];
$item = $_GET['method'];
$item = $_GET['to'];
$item = $_GET['from'];

try
{
  $api = new PhpSIP();
  $api->setProxy('sip.host.ext:5060'); 
  $api->addHeader('Event: Homer');
  $api->setMethod('OPTIONS');
  $api->setFrom('sip:1234@host.ext');
  $api->setUri('sip:1234@host.ext');
  $api->setUserAgent('HOMER SIPCAPTURE');
  $res = $api->send();

  echo "response: $res\n";
  
} catch (Exception $e) {
  
  echo $e;
}

?>
