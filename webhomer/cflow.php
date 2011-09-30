<?php

/*
 *        App: Homer's Own CallFlow 
 *        Author: Alexandr Dubovikov <alexandr.dubovikov@gmail.com>
 *
*/


// cflow.php

include("class.db.php");
$db = new homer();

if($db->logincheck($_SESSION['loggedin'], "logon", "password", "useremail") == false){
        //do something if NOT logged in. For example, redirect to login page or display message.
        header("Location: index.php\r\n");
        exit;
}

define(_HOMEREXEC, "1");


/* clear cflow image cache in PCAPDIR */
 if (CFLOW_CLEANUP != 0) {
$expiretime=86440; // default ttl 24h
$fileTypes="*.png";
foreach (glob( PCAPDIR . $fileTypes) as $tmpfile) {
        if ( (time() - (filectime($tmpfile)) ) > ($expiretime)) 
        {
        // clear old files
        unlink($tmpfile);
        }
    }
}


/* My Nodes */
$mynodeshost = array();
$mynodesname = array();
$nodes = $db->getAliases('nodes');
foreach($nodes as $node) {
        $mynodeshost[$node->id] = $node->host;
        $mynodesname[$node->id] = $node->name;
}

$arrow_step=40;
$host_step=200;

function imagelinethick($image, $x1, $y1, $x2, $y2, $color, $thick = 1)
{
   $t = $thick / 2 - 0.5;
   if ($x1 == $x2 || $y1 == $y2) {
       return imagefilledrectangle($image, round(min($x1, $x2) - $t), round(min($y1, $y2) - $t), round(max($x1, $x2) + $t), round(max($y1, $y2) + $t), $color);
   }
   $k = ($y2 - $y1) / ($x2 - $x1); //y = kx + q
   $a = $t / sqrt(1 + pow($k, 2));
   $points = array(
       round($x1 - (1+$k)*$a), round($y1 + (1-$k)*$a),
       round($x1 - (1-$k)*$a), round($y1 - (1+$k)*$a),
       round($x2 + (1+$k)*$a), round($y2 - (1-$k)*$a),
       round($x2 + (1-$k)*$a), round($y2 + (1+$k)*$a),
   );
   imagefilledpolygon($image, $points, 4, $color);
   return imagepolygon($image, $points, 4, $color);
}

function arrow ( $image, $color, $x, $y, $d = -1)
{

  global $arrow_step;

  $values = array (
           $x,  $y ,
           $x + 5 * $d,  $y - 3 ,
           $x + 3 * $d,  $y ,
           $x + 5 * $d,  $y + 3
           );

  $y+=$arrow_step;

 imagefilledpolygon($image, $values, 4, $color);
 return  imagepolygon($image, $values, 4, $color);

}

//Temporally tnode == 1
$tnode=1;
$option = array(); //prevent problems

// Check if Table is set
if ($table == NULL) { $table="sip_capture"; }

//$cid="1234567890";

$cid = getVar('cid', NULL, 'get', 'string');
$cid2 = getVar('cid2', NULL, 'get', 'string');

//Make image
$where = "callid = '".$cid."'";
if(isset($cid2)) $where .= " OR callid='".$cid2."'";

$localdata=array();

//if($db->dbconnect_homer($mynodeshost[$tnode])) {
if(!$db->dbconnect_homer(NULL))
{
    //No connect;
    exit;
}


if($db->dbconnect_homer(NULL)) {

                $query = "SELECT * "
                        ."\n FROM ".HOMER_TABLE
                        ."\n WHERE ".$where." limit 100";
		
                $rows = $db->loadObjectList($query);
        }


//$query="SELECT * FROM $table WHERE $where order by micro_ts limit 100;";
$rows = $db->loadObjectList($query);
foreach($rows as $data) {

  $localdata[] = $data;
  $hosts[$data->source_ip] = 1;
  $hosts[$data->destination_ip] = 1;


  //Check user agent and generate type of UAC
  //Better to make it in DB.

 // SIP SWITCHES

 if(preg_match('/asterisk/i', $data->user_agent)) {
     $uac[$data->source_ip] = "asterisk";
     $uac[$data->user_agent] = $data->user_agent;
 }
 else if(preg_match('/FreeSWITCH/i', $data->user_agent)) {
     $uac[$data->source_ip] = "freeswitch";
     $uac[$data->user_agent] = $data->user_agent;
 }
 else if(preg_match('/kamailio|openser|opensip|sip-router/i', $data->user_agent)) {
     $uac[$data->source_ip] = "openser";
     $uac[$data->user_agent] = $data->user_agent;
 }
 else if(preg_match('/softx/i', $data->user_agent)) {
     $uac[$data->source_ip] = "sipgateway";
     $uac[$data->user_agent] = $data->user_agent;
 }
 else if(preg_match('/sipXecs/i', $data->user_agent)) {
     $uac[$data->source_ip] = "sipxecs";
     $uac[$data->user_agent] = $data->user_agent;
 }

 // SIP ENDPOINTS

 else if(preg_match('/x-lite|Bria|counter-path/i', $data->user_agent)) {
     $uac[$data->source_ip] = "counterpath";
     $uac[$data->user_agent] = $data->user_agent;
 }
 else if(preg_match('/WG4k/i', $data->user_agent)) {
     $uac[$data->source_ip] = "worldgate";
     $uac[$data->user_agent] = $data->user_agent;
 }
 else if(preg_match('/Eki/i', $data->user_agent)) {
     $uac[$data->source_ip] = "ekiga";
     $uac[$data->user_agent] = $data->user_agent;
 }
 else if(preg_match('/snom/i', $data->user_agent)) {
     $uac[$data->source_ip] = "snom";
     $uac[$data->user_agent] = $data->user_agent;
 }

 else {
      $uac[$data->source_ip] = "sipgateway";
      $uac[$data->user_agent] = $data->user_agent;
 }
 //debug
 //$uac[$data->source_ip] = "snom";
 //       print_r($uac);

}

// Calculate size of image:

$size_y = count($localdata) * $arrow_step + 100; /* Y */
$size_x = count($hosts) *  $host_step - 10; /* X */

$file=time().".png";
$path=PCAPDIR.$file;
// Create the image
$im = imagecreatetruecolor($size_x, $size_y);

//Set Font
$fontFace = 'slc.ttf';
$fontSize = '8';

//Temp BGCOLOR (center of c-finder)
$bg[0] = 255;
$bg[1] = 255;
$bg[2] = 255;

$line_x1=50;
$line_y1=90;
$line_x2=30;
$line_y2 = $size_y - 5;

$arrow_x1=40;
$arrow_y1=120;
$arrow_x2=20;
$arrow_y2=60;

$d=-1;

$click = array();

$c1 = imagecolorallocate($im, $bg[0], $bg[1], $bg[2]); //Background

$color['white']    = imagecolorallocate($im, 255, 255, 255);
$color['gray1']    = imagecolorallocate($im, 220, 220, 220);
$color['gray2']    = imagecolorallocate($im, 210, 210, 210);
$color['gray3']    = imagecolorallocate($im, 120, 120, 120);
$color['black']    = imagecolorallocate($im, 0, 0, 0);
$color['blue']     = imagecolorallocate($im, 0, 0, 255);
$color['red']      = imagecolorallocate($im, 240, 0, 0);
$color['green']    = imagecolorallocate($im, 0, 200, 0);
$color['purple']   = imagecolorallocate($im, 160, 32, 240);
$color['brown']    = imagecolorallocate($im, 139, 69, 19);
$color['darkred']  = imagecolorallocate($im, 0x90, 0x00, 0x00);
$color['darkgray'] = imagecolorallocate($im, 0x90, 0x90, 0x90);
$color['navy']     = imagecolorallocate($im, 0x00, 0x00, 0x80);
$color['darknavy'] = imagecolorallocate($im, 0x00, 0x00, 0x50);

imagefilledrectangle($im, 0, 0, $size_x, $size_y, $c1);

//Generate HOSTs
foreach($hosts as $key=>$value) {

      $COORD[$key] = $line_x1;
      //Put Array
      // array_push($click,"50,180,200,205");
      if(!isset($uac[$key])) $uac[$key] = "sipgateway";      
      if(file_exists('./images/cflow/'.$uac[$key].".jpg")) {
         $ico = imagecreatefromjpeg('./images/cflow/'.$uac[$key].".jpg");
         imagecopymerge($im, $ico, $line_x1 - imagesx($ico)/2 , $line_y1 - (imagesy($ico) *1.5)  , 0, 0, imagesx($ico), imagesy($ico), 90);

      }

      imagelinethick($im, $line_x1, $line_y1, $line_x1, $line_y2, $color['gray2'], 1);
      //Put header!
      imagettftext ( $im, $fontSize, 0, $line_x1  - (strlen($key) * 3), $line_y1 - 10, $color['darknavy'], $fontFace, $key);

      if($line_x1 > $max_x) $max_x = $line_x1;

      $line_x1+=$host_step;    
}

//Vertical line
imagelinethick($im, 0, $line_y1, $line_x1 - $host_step + 60, $line_y1, $color['black'], 1);
imagelinethick($im, 0, $line_y2, $line_x1 - $host_step + 60, $line_y2, $color['black'], 1);

foreach($localdata as $data) {

  list($date, $time) = split(' ', $data->date);
  list($year, $month, $day) = split('[/.-]', $date);
  list($hour, $minute, $second) = split('[/:]', $time);

  //print "$year, $month, $day, $hour, $minute, $second\n<BR>";
  // Take Seconds
  $stamp=mktime($hour, $minute, $second, $month, $day, $year);
  $stamp=($stamp * 1000000 + $data->micro_ts);
    
  $text=$stamp;
  $tstamp =  date("H:i:s",$data->micro_ts / 1000000);

  $fromip = $data->source_ip;
  $fromport = $data->source_port;
  
  $toip = $data->destination_ip;
  $toport = $data->destination_port;

  //Direction
  if($COORD[$fromip] > $COORD[$toip]) 
  {
        $crd = $COORD[$fromip] - $host_step + 10;
        $d=1;
  }
  else 
  {
      $crd = $COORD[$toip] - $host_step + 10;
      $d = -1;
  }
  
  $max_y = $arrow_y1;
      
  //print "HREN:  $crd, $arrow_y1<br>\n";
  $vv=$crd+40;
 
  $method_text = $data->method." ".$data->reply_reason;
  if(strlen($method_text) > 15) $method_text = substr($data->method." ".$data->reply_reason, 0, 22)."...";
  
 
  //SDP ?
  $val = "content_type";
  //print_r($data);
  if(preg_match('/sdp/i', $data->content_type)) {
    $method_text .= " (SDP)";
  }

 // MSG Temperature
 if(preg_match('/^40|50/', $method_text )) {
    $msgcol = "red";
  } else if(preg_match('/^30|SUBSCRIBE|OPTIONS|NOTIFY/', $method_text)) {
    $msgcol = "purple";
  }   else if(preg_match('/^20/', $method_text)) {
    $msgcol = "green";
  } else {  $msgcol = 'blue';}

     
  imagettftext ( $im, $fontSize, 0,  $crd + 5, $arrow_y1 - 3, $color[$msgcol], $fontFace, $method_text);
  
  // Add Timestamp
  imagettftext ( $im, $fontSize-1, 0, $crd + 5, $arrow_y1 + 9, $color['gray3'], $fontFace, $tstamp);


  $cds = array();
  $cds[0] = $COORD[$fromip];
  $cds[1] = $arrow_y1+10;
  $cds[2] = $COORD[$toip];
  $cds[3] = $arrow_y1-10;
  //$cds[4] = nl2br(addslashes($data->msg));
  $cds[4] = nl2br(addslashes($data->id));
  
  $click[] = $cds;
  
  //Arrow
  imagelinethick($im, $COORD[$fromip], $arrow_y1, $COORD[$toip], $arrow_y1, $color['black'], 1);
  arrow($im, $color['blue'], $COORD[$toip], $arrow_y1, $d);
  
  //Port
  if($d == 1) { 
       $tportx = $COORD[$toip] - 40;
       $fportx = $COORD[$fromip] + 10;
       $portf = $toport;
       $portt = $fromport;
  }
  else {
       $tportx = $COORD[$toip] + 10;
       $fportx = $COORD[$fromip] - 40;    
       $portf = $fromport;
       $portt = $toport;
  }
    
  imagettftext ( $im, $fontSize, 0, $tportx, $arrow_y1 + 6, $color['gray3'], $fontFace, $portf);

  imagettftext ( $im, $fontSize, 0, $fportx, $arrow_y1 + 6, $color['gray3'], $fontFace, $portt);

  $arrow_y1+=$arrow_step;

  if(!$first) 
  {
    $first=$stamp;
    //imagelinethick($im, 12, 10, 12, 500, $black, 1);
  }  
}

// Using imagepng() results in clearer text compared with imagejpeg()
imagepng($im, $path);
imagedestroy($im);
//<area href=' vocal-bad-ack2.cap' coords='50,62,900,85'></area>
//<area href='test' coords='40,80,200,65'></area>
?>
<html>
<head>
<link href="styles/core_styles.css" rel="stylesheet" type="text/css" />
<link href="styles/form.css" rel="stylesheet" type="text/css" />
<link type="text/css" href="styles/jquery-ui-1.8.4.custom.css" rel="stylesheet" />
<script src="js/jquery-1.6.4.min.js" type="text/javascript"></script>
<script type="text/javascript" src="js/jquery-ui-1.8.16.custom.min.js"></script>
<script src="js/jquery.zoomable.js" type="text/javascript"></script>                                                    
<script language="javascript">
$(document).ready(function(){

      $('input:button').button();

      $('#image').zoomable();

    });
</script>
</head>

<p>

    <input type="button" value="+" onclick="$('#image').zoomable('zoomIn')" title="Zoom in" />
    <input type="button" value="-" onclick="$('#image').zoomable('zoomOut')" title="Zoom out" />
    <input type="button" value="Reset" onclick="$('#image').zoomable('reset')" />
</p>
<center>
<div style="overflow:hidden;width:<?php echo $size_x;?>px;height:<?php echo $size_y;?>px;">
<img border='0' src='<?echo WEBPCAPLOC.$file?>' usemap='#map' id="image">
<map name='map' id='map'>
<?php
foreach($click as $cds) {
     $cz = $cds[0].",".$cds[1].",".$cds[2].",".$cds[3];
     $messg = $cds[4];
     echo "<area shape='rect' href='javascript:popMessage(\"".$messg."\")' coords='$cz' alt='Area'></area>\n";
}

?>
</map>
</div>
</center>
</body>
</html>
