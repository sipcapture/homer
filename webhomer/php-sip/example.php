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
  $api->setProxy('sipx.qxip.net:5060'); 
  $api->addHeader('Event: homer');
  $api->setMethod('OPTIONS');
  $api->setFrom('sip:4504@qxip.net');
  $api->setUri('sip:4504@qxip.net');
  $api->setUserAgent('HOMER SIPCAPTURE');
  $res = $api->send();

  echo "response: $res\n";
  
} catch (Exception $e) {
  
  echo $e;
}

?>
