<?php
/*
 * HOMER Web Interface
 * App: Homer's Stats generator (Stats/Methods)
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
echo '<script type="text/javascript" src="js/highstock.js"></script>';
} else { $included = 0; }


date_default_timezone_set(CFLOW_TIMEZONE);
$offset = STAT_OFFSET;
$xhours = STAT_RANGE;

?>


		<div id="chart1" style="min-width: 380px; width: 99%; margin-left: 1px; float: left; height: 200px"></div>
		<div id="control-graph">
                <select id="timer-graph"  style="width: 45; border: 0; float: right;  margin-left: 5; height: 15;" >
                        <option value="0">0</option>
                        <option value="5">5</option>
                        <option value="10">10</option>
                        <option value="15">15</option>
                </select>
                <button id="refresh-graph" style="width: 60; border: 0; background: #fff; float: right; margin: 0 0 9 0;"  class="ui-button ui-widget2 ui-corner-all">refresh</button>
            </div>

<script type="text/javascript">

$ = jQuery;

$('#refresh-graph').click(function(){

<?php

if (inet_pton($_SERVER['SERVER_NAME']) == false) {
        $localhomer = $_SERVER['SERVER_NAME'];
} else {
        $localhomer = "[".$_SERVER['SERVER_NAME']."]";
}
if(!defined('APIURL')) define('APIURL', "http://".$localhomer);

$uri = APIURL.APILOC;

// INVITES
$request = $uri."api.php?task=statscount&method=INVITE&hours=".$xhours;
$jsondata = file_get_contents($request);
$response = json_decode($jsondata, true);
//print_r( $response);
foreach($response as $entry){
        foreach($entry as $data){
	$newtime = (strtotime($data['from_date']." ".$offset));
        $secondes[] = '['.$newtime.'000, '.$data['total'].']';
        }
}

// REGISTRATIONS
$request =  $uri."api.php?task=statscount&method=REGISTER&hours=".$xhours;
$jsondata = file_get_contents($request);
$response = json_decode($jsondata, true);
//print_r( $response);
foreach($response as $entry){
        foreach($entry as $data){
	$newtime = (strtotime($data['from_date']." ".$offset));
	$auth = $data['auth']; $pass = $data['completed'];  $total = $data['total']; $failed = ($total - $pass);
        $sip401[] = '['.$newtime.'000, '.$failed.']';
        }
}

// GENERAL FLOW
$request =  $uri."api.php?task=statscount&method=CURRENT&hours=".$xhours;
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


var chart1 = new Highcharts.Chart({

      chart: {

         renderTo: 'chart1',
	 backgroundColor: null,
	 zoomType: 'x',  
	 events: {
		selection: function(event) {
		if(event.xAxis) {
			var included = <?php echo $included; ?>;
		 if(included==0) {
		    var from_date = Highcharts.dateFormat('%d-%m-%Y', event.xAxis[0].min);
                    var to_date = Highcharts.dateFormat('%d-%m-%Y', event.xAxis[0].max);
                    var from_time = Highcharts.dateFormat('%H:%M:%S', event.xAxis[0].min);
                    var to_time = Highcharts.dateFormat('%H:%M:%S', event.xAxis[0].max);
		    //alert(from_date+' '+from_time+'/ '+to_date+' '+to_time);
			 // Set from/to Time based on graph click
                        document.getElementById('from_time').value = from_time;
                        document.getElementById('to_time').value = to_time;
                        // set date
                        document.getElementById('from_date').value = from_date;
                        document.getElementById('to_date').value = to_date;
			// document.getElementById('resetZoom');
			}
		     	}
            	}
	}
	
	// event end

      },

      title: {

         text: 'Total Packets: <?php echo $sipTOT; ?>'

      },

      legend: {

        enabled: false

      },

      xAxis: [{

	type: 'datetime',
	//min: (Date().getDate() -1),
      }],

      yAxis: [{ // Primary yAxis

         labels: {

            formatter: function() {

               return this.value +' ';

            },

            style: {

               color: '#DDDF0D',
	      	    fontWeight: 'bold',
	            fontSize: '12px',
	            fontFamily: 'Trebuchet MS, Verdana, sans-serif'

            },

            align: 'left',

            x: 0,

            y: -2

         },

          showFirstLabel: false,

         title: {

            text: 'Packets/Calls',

            style: {

               color: '#89A54E'

            }

         }

      }, { // Secondary yAxis

         title: {

            text: 'Sessions/Packets',

            style: {

               color: '#4572A7'

            },

//	 max: '10000',

         },

         labels: {

            align: 'right',

            x: 0,

            y: -2,

            formatter: function() {

               return this.value +' ';

            },

            style: {

               color: '#4572A7',
	 	    fontWeight: 'bold',
	            fontSize: '12px',
	            fontFamily: 'Trebuchet MS, Verdana, sans-serif'

            }

         },

        showFirstLabel: false,

         opposite: true

      }],

      tooltip: {

         formatter: function() {

            if (this.series.name == 'Calls') {

              return '<b>Calls:</b> '+ this.y // + '<br>@'+Date(this.x);

            }  else  if (this.series.name == 'User-Agent') {

              return false;


            } else {

              return '<b>' + this.y + ' </b> ' + this.series.name + '';


            }

         }

      },
// click start (EXPERIMENTAL)
 plotOptions: {
         series: {
            cursor: 'pointer',
            point: {
               events: {
                  click: function() {
		     // if(event) {
			   var included = <?php echo $included; ?>;
			if(included==0) {
                        var a = new Date(this.x);
                        var date = pad(a.getDate())+'-'+(a.getMonth()+1)+'-'+a.getFullYear();

                        var hours = a.getUTCHours();
                        var minutes = a.getUTCMinutes();
                        var seconds = a.getUTCSeconds();
			var span = 5;
			// adjust timespan
			if ((minutes - span) < 0) { var tominutes = 60 - span; var tohours = (hours -1); }
                        else { var tominutes = (minutes - span); tohours = hours; }
			if ((minutes + span) >= 60) { var minutes = 0 + span; var hours = (hours +1); }
                        else { var minutes = (minutes + span); tohours = hours; }
                        var to_time= pad(hours)+':'+pad(minutes)+':00';
                        var from_time= pad(tohours) +':'+pad(tominutes)+':00';
			// Set from/to Time based on graph click
			document.getElementById('from_time').value = from_time;
			document.getElementById('to_time').value = to_time;
			// set date
			document.getElementById('from_date').value = date;
			document.getElementById('to_date').value = date;

			 // If CALLS set/clear fields accordingly
			 if (this.series.name == 'Calls') {
			 document.getElementById('filters_a').value = "INVITE"; 
			 //document.getElementById('method').value = "200"; 
                    	 document.getElementById('cseq').value = "%INVITE";
			 // if (minutes == 55) { var minutes = 5; } else { var minutes = minutes +5;}
			 var span = 10;
			 } else if (this.series.name == 'Auth Failures') {
			 document.getElementById('filters_a').value = "REGISTER";
			 document.getElementById('cseq').value = "";
			 document.getElementById('method').value = "401";
			 } else {  
			 document.getElementById('filters_a').value = "";
			 document.getElementById('method').value = "";
			 document.getElementById('cseq').value = "";
			 }
			}
		    // }

                  }
               }
            },
         }
      },
// click end

      series: [

	{

         name: 'Sessions',

         color: '#4572A7',

//         type: 'column',
         type: 'spline',

         yAxis: 1,

	data :  [<?php echo join($sip100, ', ');?>]
      

      }, 

	{

         name: 'Auth Failures',

         color: '#FF72A7',

//         type: 'spline',
         type: 'column',

         yAxis: 1,

        data :  [<?php echo join($sip401, ', ');?>]

      }, 

	{

         name: 'Calls',

         color: '#DDDF0D',

         type: 'spline',

         data: [<?php echo join($secondes, ', ');?>]

      }]

   });
 return false;
});

jQuery(document).ready(function() {

$('#timer-graph').change(function () { clearInterval(refresh_graph); setTT(this.value); });

                        $("#refresh-graph").click();
                        var refresh_graph = 0;
                        function setTT(timer){
                                if (timer == 0) { clearInterval(refresh_graph); } else {
                                var timerx = ((timer*1000) * 60);
                                refresh_graph = setInterval(
                                function ()
                                {
                                $('#refresh-graph').click();
                                }, timerx );
                                }
							}


});

function pad(number) {
     return (number < 10 ? '0' : '') + number;
}  


</script>		

