<?php
/*
 * HOMER Web Interface
 * App: Homer's Stats generator (Alternative Version)
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

if (!defined('APILOC')) {
$included = 1;
include('../../configuration.php');
//echo '<script type="text/javascript" src="js/jquery.flot.js"></script>';
} else { $included = 0; }


date_default_timezone_set(CFLOW_TIMEZONE);
$offset = STAT_OFFSET;
$xhours = STAT_RANGE;
if(isset($_GET['range']) && intval($_GET['range']) <= 96 &&  intval($_GET['range']) >= 1) $xhours = intval($_GET['range']);

$to_date = date("Y-m-d H:i:s", time() );  
$from_date = date("Y-m-d H:i:s", time() - ($xhours/60/60) );

echo $from_time." -> ".$to_time;
?>


		<div id="chart1" style="min-width: 360px; width: 99%; margin-left: 1px; float: center; height: 220px"></div>



<script type="text/javascript">



<?php

if ( substr_count($_SERVER['SERVER_NAME'],":") < 2 ) {
        $localhomer = $_SERVER['SERVER_NAME'];
} else {
        $localhomer = "[".$_SERVER['SERVER_NAME']."]";
}
if(!defined('APIURL')) define('APIURL', "http://".$localhomer);

$uri = APIURL.APILOC;

$secondes = array();
$asr = array();
$sip100 = array();
$sip401 = array();

// INVITES
$request = $uri.'<?php echo APILOC;?>statistic/method/all?data={"method":"INVITE","from_date":"'.$from_date.'","to_date":"'.$to_date.'"}';
//$request = $uri."api.php?task=statscount&method=INVITE&hours=".$xhours;
$jsondata = file_get_contents($request);
$response = json_decode($jsondata, true);
//print_r( $response);
foreach($response as $entry){
        foreach($entry as $data){
	$newtime = (strtotime($data['from_date']." ".$offset));
        $secondes[] = '['.$newtime.'000, '.$data['total'].']';
        $asr[] = '['.$newtime.'000, '.$data['asr'].']';
        }
}

// REGISTRATIONS
$request = $uri.'<?php echo APILOC;?>statistic/method/all?data={"method":"REGISTER","from_date":"'.$from_date.'","to_date":"'.$to_date.'"}';
// $request =  $uri."api.php?task=statscount&method=REGISTER&hours=".$xhours;
$jsondata = file_get_contents($request);
$response = json_decode($jsondata, true);
//print_r( $response);
foreach($response as $entry){
        foreach($entry as $data){
	$newtime = (strtotime($data['from_date']." ".$offset));
	$auth = $data['auth']; $pass = $data['completed']; $total = $data['total'];
	$failed = $total - $pass; 
                	$sip401[] = '['.$newtime.'000, '.$failed.']';
        }
}

// GENERAL FLOW
$request = $uri.'<?php echo APILOC;?>statistic/method/all?data={"method":"ALL","from_date":"'.$from_date.'","to_date":"'.$to_date.'"}';
// $request =  $uri."api.php?task=statscount&method=CURRENT&hours=".$xhours;
$jsondata = file_get_contents($request);
$response = json_decode($jsondata, true);
//print_r( $response);
foreach($response as $entry){
        foreach($entry as $data){
        $newtime = (strtotime($data['from_date']." ".$offset));
                $sip100[] = '['.$newtime.'000, '.$data['total'].']';
        }
}

// GRANMASTERTOTAL
$request =  $uri."api.php?task=statscount&method=ALL";
$jsondata = file_get_contents($request);
$response = json_decode($jsondata, true);
//print_r( $response);
foreach($response as $entry){
        foreach($entry as $data){
                $sipTOT = $data['total'];
        }
}


?>

$ = jQuery;

jQuery(document).ready(function() {

 var d1 = [<?php echo join($secondes, ', ');?>];
 var d2 = [<?php echo join($sip100, ', ');?>];
 var d3 = [<?php echo join($sip401, ', ');?>];
 var allp = "<?php if (isset($sipTOT)) echo $sipTOT; ?>";
 var asr = [<?php echo join($asr, ', ');?>];


	$.plot($("#chart1"), 
		[ 
		{ data: d2, label: "Packets", lines: { show: true, fill: true } },
             	{ data: d1, label: "Calls", yaxis: 2, lines: {show: true}, points: { show: true } }, 
             	{ data: d3, label: "AuthFail", yaxis: 1,  bars: { show: true } },
             	{ data: asr, label: "ASR", yaxis: 1,  lines: { show: true, steps: true }, 
		  color: "rgb(30, 180, 20)", threshold: { below: 60, color: "rgb(200, 20, 30)" } }
		],
           { 
               xaxes: [ { mode: 'time' } ],
               yaxes: [ { position: 'left' }, { position: 'right' }],
  	       legend: { position: "nw", margin: 10, show: 'true' },
  	       grid: { borderWidth: 0, clickable: true }
           });

    $('#chart1').bind("plotclick", function (event, pos, item) {
	if (item) {
	   if (document.getElementById('from_time')) {
            //$("#clickdata").text("You clicked point " + item.dataIndex + " in " + item.series.label + ".");
		var a = new Date(item.datapoint[0]-160000);
		var b = new Date(item.datapoint[0]+360000);
                var from_date = pad(a.getDate())+'-'+(a.getMonth()+1)+'-'+a.getFullYear();
                var to_date = pad(b.getDate())+'-'+(b.getMonth()+1)+'-'+b.getFullYear();
		var from_time = pad(a.getUTCHours())+':'+pad(a.getUTCMinutes())+':'+pad(a.getUTCSeconds());
		var to_time = pad(b.getUTCHours())+':'+pad(b.getUTCMinutes())+':'+pad(b.getUTCSeconds());
		// Set from/to Time based on graph click
                document.getElementById('from_time').value = from_time;
                document.getElementById('to_time').value = to_time;
                // set date
                document.getElementById('from_date').value = from_date;
                document.getElementById('to_date').value = to_date;
		//alert(date+' '+time+' to:'+to_time);
	   }
		
        }
     });

     $('#chart1').append('<div style="position:absolute;left:40%;top:10px;color:#666;font-size:small">Captured Frames: '+allp+'</div>');


});



function pad(number) {
     return (number < 10 ? '0' : '') + number;
}  


</script>		

