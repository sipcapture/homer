<?php
/*
 * HOMER Web Interface
 * App: Homer's Stats generator (User-Agents)
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

//if ($_GET['hours']) {$hours = $_GET['hours'];} else {$hours= 48;}
$API = APILOC;
if ($API == 'APILOC') {
$included = 1;
include('../../configuration.php');
echo '<script type="text/javascript" src="js/highstock.js"></script>';
} else { $included = 0;}


// Server hook
//include('configuration.php');
$hours = STAT_RANGE;

?>



		<!-- <script type="text/javascript" src="../stats/js/modules/exporting.js"></script> -->
		<div id="chart5" style="float: left; min-width: 150px; width: 49%; min-height: 200px;"></div>
		<div id="chart4" style="float: right; min-width: 150px; width: 49%; min-height: 100px;"></div>
		<div id="chart6" style="float: right; min-width: 150px; width: 49%; min-height: 100px;"></div>
		<div id="control-stats">
                <select id="timer-stats"  style="width: 45; border: 0; float: right;  margin-left: 5; height: 15;" >
                        <option value="0">0</option>
                        <option value="5">5</option>
                        <option value="10">10</option>
                        <option value="15">15</option>
                </select>
                <button id="refresh-stats" style="width: 60; border: 0; background: #fff;  float: right; margin: 0 0 9 0;"  class="ui-button ui-widget2 ui-corner-all">refresh</button>
            	</div>

<script type="text/javascript">

$ = jQuery;

$('#refresh-stats').click(function() {

$('#live-stats').html('');

<?php

if ( substr_count($_SERVER['SERVER_NAME'],":") < 2 ) {
        $localhomer = $_SERVER['SERVER_NAME'];
} else {
        $localhomer = "[".$_SERVER['SERVER_NAME']."]";
}
if(!defined('APIURL')) define('APIURL', "http://".$localhomer);

$uri = APIURL.APILOC;

$request = $uri."api.php?task=statscount&method=INVITE&measure=1&hours=".$hours;
$jsondata = file_get_contents($request);
$response = json_decode($jsondata, true);
//print_r( $response);
foreach($response as $entry){
        foreach($entry as $qdat){
	if ($qdat['avg(asr)'] >= 70) {$colASR="#8dc141"; } else if($qdat['avg(asr)'] >=50) {$colASR="#fdc141";} else {$colASR='#8d0101';}
	if ($qdat['avg(ner)'] >= 80) {$colNER="#8dc140"; } else if($qdat['avg(ner)'] >=50) {$colNER="#fdc140";} else {$colNER='#8d0100';}
  	$sipASR[] = ' '.$qdat['avg(asr)'].', ';
        $sipNER[] = ' '.$qdat['avg(ner)'].', ';
        $callTOT[] = ' '.$qdat['sum(total)'].' ';
        $callOK[] = ' '.$qdat['sum(completed)'].' ';
        $callKO[] = ' '.$qdat['sum(uncompleted)'].' ';
        }
}

$request = $uri."api.php?task=statscount&method=REGISTER&measure=1&hours=".$hours;
$jsondata = file_get_contents($request);
$response = json_decode($jsondata, true);
//print_r( $response);
foreach($response as $entry){
        foreach($entry as $qdat){
        $regOK[] = ' '.$qdat['sum(completed)'].' ';
        $regKO[] = ' '.$qdat['sum(uncompleted)'].' ';
        }
}

?>


// NER
var chart6 = new Highcharts.Chart({
      chart: {
         renderTo: 'chart6',
         backgroundColor: 'transparent',
         defaultSeriesType: 'bar'
      },
      title: {
//         text: 'Last <?php echo $hours; ?>h',
	text: 'N.E.R.',
	style: {
         font: 'bold 9px Verdana, sans-serif'
        }

      },
      xAxis: {

         categories: [' '],
         title: {
            text:  null,
         }
      },
      yAxis: {
         min: 0,
	 max: 100,
         title: {
            text: '',
         }
      },
      tooltip: {
         formatter: function() {
            return ''+
                this.series.name +': '+ this.y +'';
         }
      },
      plotOptions: {
         bar: {
            dataLabels: {
               enabled: false
            }
         }
      },
      legend: {
         layout: 'vertical',
         align: 'right',
         verticalAlign: 'top',
         x: -100,
         y: 100,
         floating: true,
         borderWidth: 1,
         backgroundColor: '#ffffff',
         shadow: true
      },
      credits: {
         enabled: false
      },
           series: [{
         name: 'NER',
	 color: '<?php echo $colNER; ?>',
         data: [ <?php echo join($sipNER, ', '); ?> ]
      }]
   });

// ASR
var chart4 = new Highcharts.Chart({
      chart: {
         renderTo: 'chart4',
         backgroundColor: 'transparent',
         defaultSeriesType: 'bar'
      },
      title: {
//         text: 'Last <?php echo $hours; ?>h',
	text: 'A.S.R.',
	style: {
         font: 'bold 9px Verdana, sans-serif'
        }

      },
      xAxis: {

         categories: [' '],
         title: {
            text:  null,
         }
      },
      yAxis: {
         min: 0,
	 max: 100,
         title: {
            text: '',
         }
      },
      tooltip: {
         formatter: function() {
            return ''+
                this.series.name +': '+ this.y +'';
         }
      },
      plotOptions: {
         bar: {
            dataLabels: {
               enabled: false
            }
         }
      },
      legend: {
         layout: 'vertical',
         align: 'right',
         verticalAlign: 'top',
         x: -100,
         y: 100,
         floating: true,
         borderWidth: 1,
         backgroundColor: '#ffffff',
         shadow: true
      },
      credits: {
         enabled: false
      },
           series: [{
         name: 'ASR',
	 color: '<?php echo $colASR; ?>',
         data: [ <?php echo join($sipASR, ', '); ?> ]
      }]
   });


// CALLS
var chart5 = new Highcharts.Chart({
chart: {
         renderTo: 'chart5',
         backgroundColor: 'transparent',
         defaultSeriesType: 'column'
      },
      title: {
         text: ' '
      },
      xAxis: {
         categories: ['Calls', 'Registrations']
      },
      yAxis: {
         min: 0,
         title: {
            text: 'Success Rate %'
         }
      },
      tooltip: {
         formatter: function() {
            return ''+
                this.series.name +': '+ 
		//this.y 
		'('+ Math.round(this.percentage) +'%)';
         }
      },
      plotOptions: {
         column: {
            stacking: 'percent',
         }

      },
      legend: {
         layout: 'vertical',
         align: 'right',
         verticalAlign: 'bottom',
         x: -100,
         y: 100,
         floating: true,
         borderWidth: 1,
         backgroundColor: '#ffffff',
         shadow: true
      },
      credits: {
         enabled: false
      },


      series: [{
         name: 'Reject',
         data: [ 0, 0 ]
      }, {
         name: 'Failed',
         data: [ <?php echo join($callKO, ', ');?>,  <?php echo join($regKO, ', ');?>]
      }, {
         name: 'Completed',
         data: [ <?php echo join($regOK, ', ');?>,  <?php echo join($regOK, ', ');?>]
	}]

   });

return false;

});

jQuery(document).ready(function() {

		$('#timer-stats').change(function () { clearInterval(refresh_stats); setTT(this.value); });
                        $("#refresh-stats").click();
                        var refresh_stats = 0;
                        function setTT(timer){
                                if (timer == 0) { clearInterval(refresh_stats); } else {
                                var timerx = ((timer*1000) * 60); // minutes
                                refresh_stats = setInterval(
                                function ()
                                {
                                $('#refresh-stats').click();
                                }, timerx );
                                }
							}
});

   

</script>		

