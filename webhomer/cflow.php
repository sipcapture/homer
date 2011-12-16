<?php
/*
 * HOMER Web Interface
 * Homer's cflow.php
 *
 * Copyright (C) 2011-2012 Alexandr Dubovikov <alexandr.dubovikov@gmail.com>
 * Copyright (C) 2011-2012 Lorenzo Mangani <lorenzo.mangani@gmail.com>
 *
 * The Initial Developers of the Original Code are
 *
 * Alexandr Dubovikov <alexandr.dubovikov@gmail.com>
 * Lorenzo Mangani <lorenzo.mangani@gmail.com>
 *
 * This program is free software; you can redistribute it and/or
 * modify it under the terms of the GNU General Public License
 * as published by the Free Software Foundation; either version 2
 * of the License, or (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program; if not, write to the Free Software
 * Foundation, Inc., 51 Franklin Street, Fifth Floor, Boston, MA  02110-1301, USA.
 *
*/


define(_HOMEREXEC, "1");

/* MAIN CLASS modules */
include("class/index.php");

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

$aliases = $db->getAliases();

$arrow_step=40*CFLOW_FACTOR;
$host_step=200*CFLOW_FACTOR;

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
           $x + 5*CFLOW_FACTOR * $d,  $y - 3*CFLOW_FACTOR ,
           $x + 3*CFLOW_FACTOR * $d,  $y ,
           $x + 5*CFLOW_FACTOR * $d,  $y + 3*CFLOW_FACTOR
           );

  $y+=$arrow_step;

 imagefilledpolygon($image, $values, 4, $color);
 return  imagepolygon($image, $values, 4, $color);

}

//Temporally tnode == 1
$tnode=1;
$option = array(); //prevent problems

//$cid="1234567890";

//callid_aleg
$cid = getVar('cid', NULL, 'get', 'string');
$b2b = getVar('b2b', 0, 'get', 'int');
$popuptype = getVar('popuptype', 1, 'get', 'int');
if(BLEGDETECT == 1) $b2b =1;

//Crop Search Parameters, if any
$flow_from_date = getVar('from_date', NULL, 'get', 'string');
$flow_from_time = getVar('from_time', NULL, 'get', 'string');
$flow_to_time = getVar('to_time', NULL, 'get', 'string');
$flow_to_date = getVar('to_date', NULL, 'get', 'string');

if (isset($flow_from_date, $flow_from_time, $flow_to_time, $flow_to_date))
{
  $ft = date("Y-m-d H:i:s", strtotime($flow_from_date." ".$flow_from_time));
  $tt = date("Y-m-d H:i:s", strtotime($flow_to_date." ".$flow_to_time));
  $where = "(`date` BETWEEN '$ft' AND '$tt' )";
}

/* Prevent break SQL */
if(isset($where)) $where.=" AND ";

//if($db->dbconnect_homer($mynodeshost[$tnode])) {
if(!$db->dbconnect_homer(NULL))
{
    //No connect;
    exit;
}

/* CID */
$where.="( callid = '".$cid."'";

/* Detect second B-LEG ID */
if($b2b) {
    if(BLEGCID == "x-cid") {
          $query = "SELECT callid FROM ".HOMER_TABLE
                  ."\n WHERE ".$where." AND callid_aleg='".$cid."'";
          $cid_aleg = $db->loadResult($query);    
    }
    else if (BLEGCID == "-0") $cid_aleg = $cid.BLEGCID;    
    else $cid_aleg = $cid;
  
    $where .= " OR callid='".$cid.BLEGCID."'";
}
$where .= ") ";

$localdata = array();
$rtpinfo   = array();

$query = "SELECT * "
          ."\n FROM ".HOMER_TABLE
          ."\n WHERE ".$where." order by micro_ts ASC limit 100";

$rows = $db->loadObjectList($query);

if(count($rows)==0) {
    echo "No data found!";
    exit;
}

$querytd = "SELECT TIMEDIFF(max(date),min(date)) as tot_dur "
          ."\n FROM ".HOMER_TABLE
          ."\n WHERE ".$where;
$totdurs = $db->loadObjectList($querytd);
$totdur = $totdurs[0]->tot_dur;

$statuscall = 0;

//$query="SELECT * FROM $table WHERE $where order by micro_ts limit 100;";
$rows = $db->loadObjectList($query);
foreach($rows as $data) {
  
  /* LOCAL RESOLV */
  foreach($aliases as $alias) {
            if($alias->host == $data->source_ip) $data->source_ip = $alias->name;
            if($alias->host == $data->destination_ip) $data->destination_ip = $alias->name;
  }

  $localdata[] = $data;  
  
  if(preg_match('/[4-6][0-9][0-9]/',$data->method)) $statuscall = 1;
  else if($data->method == "CANCEL") $statuscall = 2;
  else if($data->method == "BYE") $statuscall = 3;
  else if($data->method == "200" && preg_match('/INVITE/',$data->cseq)) $statuscall = 4;
  else if(preg_match('/[3][0-9][0-9]/',$data->method)) $statuscall = 5;
  
  $hosts[$data->source_ip] = 1;
  $hosts[$data->destination_ip] = 1;
  
  /* RTP INFO */
  if(preg_match('/=/',$data->rtp_stat)) {
	$newArray = array();
	$tmparray = explode(",", $data->rtp_stat);
	foreach ($tmparray as $lineNum => $line) {
		list($key, $value) = explode("=", $line);
		$newArray[$key] = $value;
	}			
	$rtpinfo[] = $newArray;
  }  

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
 

}

// Calculate size of image:

$size_y = count($localdata) * $arrow_step + 100*CFLOW_FACTOR; /* Y */
$size_x = count($hosts) *  $host_step - 10*CFLOW_FACTOR; /* X */

$file=time().".png";
$path=PCAPDIR.$file;
// Create the image
$im = imagecreatetruecolor($size_x, $size_y);

//Set Font
$fontFace = './slc.ttf';
$fontSize = 8*CFLOW_FACTOR;

//Temp BGCOLOR (center of c-finder)
$bg[0] = 255;
$bg[1] = 255;
$bg[2] = 255;

$line_x1=50*CFLOW_FACTOR;
$line_y1=90*CFLOW_FACTOR;
$line_x2=30*CFLOW_FACTOR;
$line_y2 = $size_y - 5*CFLOW_FACTOR;

$arrow_x1=40*CFLOW_FACTOR;
$arrow_y1=120*CFLOW_FACTOR;
$arrow_x2=20*CFLOW_FACTOR;
$arrow_y2=60*CFLOW_FACTOR;

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

      imagelinethick($im, $line_x1, $line_y1, $line_x1, $line_y2, $color['gray2'], 2*CFLOW_FACTOR);
      //Put header!
      imagettftext ( $im, $fontSize, 0, $line_x1  - (strlen($key) * 3*CFLOW_FACTOR), $line_y1 - 10, $color['darknavy'], $fontFace, $key);

      if($line_x1 > $max_x) $max_x = $line_x1;

      $line_x1+=$host_step;    
}

//Vertical line
imagelinethick($im, 0, $line_y1, $line_x1 - $host_step + 60*CFLOW_FACTOR, $line_y1, $color['black'], 1*CFLOW_FACTOR);
imagelinethick($im, 0, $line_y2, $line_x1 - $host_step + 60*CFLOW_FACTOR, $line_y2, $color['black'], 1*CFLOW_FACTOR);

foreach($localdata as $data) {

  list($date, $time) = split(' ', $data->date);
  list($year, $month, $day) = split('[/.-]', $date);
  list($hour, $minute, $second) = split('[/:]', $time);

  //print "$year, $month, $day, $hour, $minute, $second\n<BR>";
  // Take Seconds
  $stamp=mktime($hour, $minute, $second, $month, $day, $year);
  $stamp=($stamp * 1000000 + $data->micro_ts);
    
  $text=$stamp;
  date_default_timezone_set(CFLOW_TIMEZONE);
  //$tstamp =  date("Y-m-d H:i:s T",$data->micro_ts / 1000000);
  $timestamp = floor($data->micro_ts / 1000000);
  $milliseconds = round( $data->micro_ts  - ($timestamp * 1000000) );
  $tstamp =  date("Y-m-d H:i:s.".$milliseconds." T",$data->micro_ts / 1000000);


  $fromip = $data->source_ip;
  $fromport = $data->source_port;
  
  $toip = $data->destination_ip;
  $toport = $data->destination_port;

  //Direction
  if($COORD[$fromip] > $COORD[$toip]) 
  {
	 if (CFLOW_DIRECTION == 0 ) { 
				        $crd = $COORD[$fromip] - $host_step + 10;
					$d = 1; 

				    } else { 

				        $crd = $COORD[$fromip] - $host_step + 10;
					$d = -1; 
				    }
  }
  else 
  {
	 if (CFLOW_DIRECTION == 0 ) { 
				        $crd = $COORD[$toip] - $host_step + 10;
					$d = -1; 

				    } else { 

					$crd = $COORD[$toip] - $host_step + 10;
					$d = 1; 
				    }
  }
  
  $max_y = $arrow_y1;
      
  //print "HREN:  $crd, $arrow_y1<br>\n";
  $vv=$crd+40*CFLOW_FACTOR;
 
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

     
  imagettftext ( $im, $fontSize, 0,  $crd + 5, $arrow_y1 - 3*CFLOW_FACTOR, $color[$msgcol], $fontFace, $method_text);
  
  // Add Timestamp
  imagettftext ( $im, $fontSize-1, 0, $crd + 5, $arrow_y1 + 9*CFLOW_FACTOR, $color['gray3'], $fontFace, "[".$tstamp."]");


  $cds = array();
  $cds[0] = $COORD[$fromip];
  $cds[1] = $arrow_y1+10;
  $cds[2] = $COORD[$toip];
  $cds[3] = $arrow_y1-10;
  //$cds[4] = nl2br(addslashes($data->msg));
  $cds[4] = nl2br(addslashes($data->id));
  $cds[5] = $data->date;

  
  $click[] = $cds;
  
  //Arrow
  imagelinethick($im, $COORD[$fromip], $arrow_y1, $COORD[$toip], $arrow_y1, $color['black'], 1*CFLOW_FACTOR);

  if (CFLOW_DIRECTION == 0 ) {
  arrow($im, $color['blue'], $COORD[$toip], $arrow_y1, $d);
  			    } else {
  arrow($im, $color['blue'], $COORD[$fromip], $arrow_y1, $d);

			}
  //Port
  if($d == 1) { 
     if (CFLOW_DIRECTION == 0 ) {
        $tportx = $COORD[$toip] - 40*CFLOW_FACTOR;
        $fportx = $COORD[$fromip] + 10*CFLOW_FACTOR;
	} else {
	$tportx = $COORD[$toip] + 10*CFLOW_FACTOR;
        $fportx = $COORD[$fromip] - 40*CFLOW_FACTOR;
	}
       $portf = $toport;
       $portt = $fromport;
  }
  else {
     if (CFLOW_DIRECTION == 0 ) {
	       $tportx = $COORD[$toip] + 10*CFLOW_FACTOR;
	       $fportx = $COORD[$fromip] - 40*CFLOW_FACTOR;    
	} else {
	       $tportx = $COORD[$toip] - 40*CFLOW_FACTOR;
         $fportx = $COORD[$fromip] + 10*CFLOW_FACTOR;
	}
         
         $portt = $fromport;
         $portf = $toport;
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
$winid = rand(1111, 9999);

/* color call status */
if($statuscall == 1) $statuscolor="red";
else if($statuscall == 2) $statuscolor="orange";
else if($statuscall == 3) $statuscolor="lightgreen";
else if($statuscall == 4) $statuscolor="lightblue";
else if($statuscall == 5) $statuscolor="yellow";
else $statuscolor="white";

?>
<html>
<head>
<link href="styles/core_styles.css" rel="stylesheet" type="text/css" />
<link href="styles/form.css" rel="stylesheet" type="text/css" />
<link type="text/css" href="styles/jquery-ui-1.8.4.custom.css" rel="stylesheet" />

<?php if($popuptype == 2): ?>
<script src="js/homer.js" type="text/javascript"></script>
<script src="js/jquery-1.6.4.min.js" type="text/javascript"></script>
<script type="text/javascript" src="js/jquery-ui-1.8.16.custom.min.js"></script>
<?php endif; ?>
<title>CallFlow <?php echo $cid;?></title>
<script src="js/jquery.zoomable.js" type="text/javascript"></script>                                                    
<script language="javascript">
$(document).ready(function(){

      $('input:button').button();
      $('#rtpinfo<?php echo $winid; ?>').hide();
      $('#image<?php echo $winid; ?>').zoomable();

//      $(this).find('a.ui-dialog-titlebar-close').parent().append( $('#ybuttons') );

// Style buttons
//$('#s2').input({ icons: { primary: "ui-icon-locked" } });

    });
</script>
</head>

<div id="ybuttons" align="center" style="margin-right: 5%; width: 100%;">
    <input id="z1" type="button" value="+" onclick="$('#image<?php echo $winid; ?>').zoomable('zoomIn')" title="Zoom in"  style="background: transparent;" />
    <input id="z2" type="button" value="-" onclick="$('#image<?php echo $winid; ?>').zoomable('zoomOut')" title="Zoom out"  style="background: transparent;" />
    <input id="r1" type="button" value="Reset" onclick="$('#image<?php echo $winid; ?>').zoomable();$('#image<?php echo $winid; ?>').width('<?php echo $size_x;?>').height('<?php echo $size_y;?>');"  style="background: transparent;" />
<!--    <input id="s1" type="button" class="ui-state-default ui-corner-all" value="PNG" onclick="window.open('utils.php?task=saveit&cflow=<?php echo $file?>');"  style="background: transparent;"  /> -->
    <input id="s2" type="button" value="PCAP" onclick="window.open('pcap.php?cid=<?php echo $cid;?>&b2b=<?php echo $b2b; ?>');" style="background: transparent;"/>
<?php  if (isset($flow_from_date)) { ?>
    <input type="button" value="Duration: <?php echo $totdur ?>" style="opacity: 1; background: transparent; background-color: <?php echo $statuscolor; ?>" disabled />
    <input type="button" value="..." style="opacity: 1; background: transparent;" onclick="$(this).parent().parent().load('cflow.php?cid=<?php echo $cid ?>&b2b=<?php echo $b2b ?>');"/>
<?php } else {  ?>
    <input type="button" value="Duration: <?php echo $totdur ?>" style="opacity: 1; background: transparent; background-color: <?php echo $statuscolor; ?>" disabled />
<?php   }  ?>
    <input type="button" value="RTP info" style="opacity: 1; background: transparent;" onclick="$('#callflow<?php echo $winid; ?>').toggle(400);$('#rtpinfo<?php echo $winid; ?>').toggle(400);" />
</div>
<center>
<div id="callflow<?php echo $winid; ?>" style="overflow:hidden;width:<?php echo $size_x;?>px;height:<?php echo $size_y;?>px;">
<img border='0' src='<?php echo WEBPCAPLOC.$file?>' usemap='#map' id="image<?php echo $winid; ?>">
<map name='map' id='map'>
<?php

if(!defined('MESSAGE_POPUP')) $popuptype = 1;
else $popuptype = MESSAGE_POPUP;

foreach($click as $cds) {
     $cz = $cds[0].",".$cds[1].",".$cds[2].",".$cds[3];
     $messg = $cds[4];

     $fd = date("Y-m-d", strtotime($cds[5]));
     $ft = date("H:i:s", strtotime($cds[5]));

     $url = "utils.php?task=sipmessage&id=".$messg."&popuptype=".$popuptype;
     $url .= "&from_time=".$ft."&from_date=".$fd;

     echo "<area shape='rect' href='javascript:popMessage2(".$popuptype.",\"".$messg."\",\"".$url."\")' coords='$cz' alt='Area'></area>\n";
}

?>
</map>
</div>
<div id="rtpinfo<?php echo $winid; ?>">
<br>
<br>
<?php 

if(count($rtpinfo) == 0) echo "No rtp info available for this call";

//https://supportforums.cisco.com/servlet/JiveServlet/downloadBody/18784-102-3-46597/spaPhoneP-RTP-Stat_09292011.pdf
echo "<table border='1'>";
foreach ($rtpinfo as $key=>$data) {
	//PS = <packet sent>
	if(isset($data['PS'])) echo "<tr><td>Packets sent:</td><td>".$data['PS']."</td></tr>";
	//OS = <packet recieved>
	if(isset($data['OS'])) echo "<tr><td>Octets sent:</td><td>".$data['OS']."</td></tr>";
	//PR = <octet recieved>
	if(isset($data['PR'])) echo "<tr><td>Packets recieved:</td><td>".$data['PR']."</td></tr>";
	//OR = <octet recieved>
	if(isset($data['OR'])) echo "<tr><td>Octets recieved:</td><td>".$data['OR']."</td></tr>";
	//PL = <packet lost>
	if(isset($data['PL'])) {
		$perc = 0;
		if(isset($data['PR'])) $perc = floor($data['PL'] * 100 / $data['PR'] * 1000) / 1000;		
		echo "<tr><td>Packet lost:</td><td>".$data['PL']." ( $perc %)</td></tr>";		
	}
	//JI = <jitter ms>
	if(isset($data['JI'])) echo "<tr><td>Jitter ms:</td><td>".$data['JI']." ms.</td></tr>";
	//LA = <delay ms>
	if(isset($data['LA'])) echo "<tr><td>Delay ms:</td><td>".$data['LA']." ms.</td></tr>";
	//DU = <call duration seconds>
	if(isset($data['DU'])) {
		$total = $data['DU'];
		$minutes = intval(($total / 60) % 60); 
		$seconds = intval($total % 60); 
		echo "<tr><td>Call duration:</td><td>".$data['DU']." seconds. ($minutes min. $seconds sec.)</td></tr>";
	}
	//EN = <encoder>
	if(isset($data['EN'])) echo "<tr><td>Encoder:</td><td>".$data['EN']."</td></tr>";
	//DE = <decoder>
	if(isset($data['DE'])) echo "<tr><td>Decoder:</td><td>".$data['DE']."</td></tr>";
}
echo "</table>";
?>
</div>
</center>
</body>
</html>

