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

$API = APILOC;
if ($API == 'APILOC') {
$included = 1;
include('../../configuration.php');
} else { $included = 0; }


date_default_timezone_set(CFLOW_TIMEZONE);
$offset = STAT_OFFSET;
$hours = STAT_RANGE;

$uri = "http://".$_SERVER['SERVER_NAME'].APILOC;

$request = $uri."api.php?task=statscount&method=INVITE&measure=1&hours=".$hours;
$jsondata = file_get_contents($request);
$response = json_decode($jsondata, true);
//print_r( $response);
foreach($response as $entry){
        foreach($entry as $qdat){
        $sipASR[] = ' '.$qdat['avg(asr)'].', ';
        $sipNER[] = ' '.$qdat['avg(ner)'].', ';
        $callTOT[] = ' '.$qdat['avg(total)'].' ';
        $callOK[] = ' '.$qdat['avg(completed)'].' ';
        $callKO[] = ' '.$qdat['avg(uncompleted)'].' ';
        }
}

$request = $uri."api.php?task=statscount&method=REGISTER&measure=1&hours=".$hours;
$jsondata = file_get_contents($request);
$response = json_decode($jsondata, true);
//print_r( $response);
foreach($response as $entry){
        foreach($entry as $qdat){
        $regOK[] = ' '.$qdat['avg(completed)'].' ';
        $regKO[] = ' '.$qdat['avg(uncompleted)'].' ';
        }
}


?>
		<div id="trap" style="width: 99%;">
		<div id="chart5" style="float: left; min-width: 100px; width: 32%; min-height: 220px;"></div>
	        <div id="chart4" style="float: left; min-width: 100px; width: 32%; min-height: 220px;"></div>
	        <div id="chart6" style="float: left; min-width: 100px; width: 32%; min-height: 220px;"></div>
		</div>


<script type="text/javascript">

$ = jQuery;

$(document).ready(function() {

var asr1 = [ [0, <?php echo join($sipASR, ', ');?>] ];
var ner1 = [ [1, <?php echo join($sipNER, ', ');?>] ];

var cok1 = [ [0, <?php echo join($callOK, ', ');?>] ];
var cko1 = [ [1, <?php echo join($callKO, ', ');?>] ];

var rok1 = [ [0, <?php echo join($regOK, ', ');?>] ];
var rko1 = [ [1, <?php echo join($regKO, ', ');?>] ];

  $.plot($("#chart5"),
                [
                { data: asr1, label: "ASR",  bars: { show: true } },
                { data: ner1, label: "NER",  bars: { show: true } },
                ],
           {
               yaxes: [  { position: 'left' }
                      ],
		xaxes: [ { ticks: 0 } ],

	 legend: {
                position: "sw",
                backgroundOpacity: 1
                }

           });

   $.plot($("#chart4"),
                [
                { data: cok1, label: "Calls",  bars: { show: true } },
                { data: cko1, label: "Failed",  bars: { show: true }, color: '#ccc' },
                ],
           {
               yaxes: [  { position: 'left' }
                      ],
		xaxes: [ { ticks: 0 } ],

	 legend: {
                position: "sw",
                backgroundOpacity: 1
                }

           });

   $.plot($("#chart6"),
                [ 
                { data: rok1, label: "Register", bars: { show: true } },
                { data: rko1, label: "Failed",  bars: { show: true }, color: '#ccc' },
                ],
           {
               yaxes: [  { position: 'left' }
                      ],
                xaxes: [ { ticks: 0 } ],

	 legend: {
    		position: "sw",
    		backgroundOpacity: 1
  		}

           });


});



</script>		

