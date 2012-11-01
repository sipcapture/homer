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

// Server hook
include('configuration.php');
$hours = STAT_RANGE;

if(!defined('APIURL')) define('APIURL', "http://".$_SERVER['SERVER_NAME']);

$uri = APIURL.APILOC;

$request = $uri."api.php?task=statscount&method=INVITE&measure=1&hours=".$hours;
$jsondata = file_get_contents($request);
$response = json_decode($jsondata, true);
//print_r( $response);
foreach($response as $entry){
        foreach($entry as $qdat){
	if ($qdat['avg(asr)'] >= 70) {$colASR="#8dc140"; } else if($qdat['avg(asr)'] >=50) {$colASR="#fdc140";} else {$colASR='#8d0100';}
	if ($qdat['avg(ner)'] >= 80) {$colNER="#8dc140"; } else if($qdat['avg(ner)'] >=50) {$colNER="#fdc140";} else {$colNER='#8d0100';}
  	$sipASR[] = ' '.$qdat['avg(asr)'].', ';
        $sipNER[] = ' '.$qdat['avg(ner)'].', ';
        $callTOT[] = ' '.$qdat['avg(total)'].' ';
        $callOK[] = ' '.$qdat['avg(completed)'].' ';
        $callKO[] = ' '.$qdat['avg(uncompleted)'].' ';
        }
}

?>



<script type="text/javascript">

jQuery(document).ready(function() {

// ASR
var chart = new Highcharts.Chart({
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
            text:  '',
         }
      },
      yAxis: {
         min: 0,
	 max: 100,
         title: {
            text: '',
//            align: 'high'
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
         name: 'ASR',
	 color: '<?php echo $colASR; ?>',
         data: [ <?php echo join($sipASR, ', '); ?> ]
//      }, {
//         name: 'NER',
//	 color: '<?php echo $colNER; ?>',
//         data: [ <?php echo join($sipNER, ', '); ?> ]
      }]
   });
// NER
var chart = new Highcharts.Chart({
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
            text:  '',
         }
      },
      yAxis: {
         min: 0,
	 max: 100,
         title: {
            text: '',
//            align: 'high'
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
//         name: 'ASR',
//	 color: '<?php echo $colASR; ?>',
//         data: [ <?php echo join($sipASR, ', '); ?> ]
//      }, {
         name: 'NER',
	 color: '<?php echo $colNER; ?>',
         data: [ <?php echo join($sipNER, ', '); ?> ]
      }]
   });

// CALLS
var chart2 = new Highcharts.Chart({
chart: {
         renderTo: 'chart5',
         backgroundColor: 'transparent',
         defaultSeriesType: 'column'
      },
      title: {
         text: ' '
      },
      xAxis: {
         categories: ['Calls']
      },
      yAxis: {
         min: 0,
         title: {
            text: 'Call Completion'
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
         data: [ 0 ]
      }, {
         name: 'Failed',
         data: [ <?php echo join($callKO, ', ');?>]
      }, {
         name: 'Completed',
         data: [ <?php echo join($callOK, ', ');?>]
      }]

   });

});
   

</script>		

		<!-- <script type="text/javascript" src="../stats/js/modules/exporting.js"></script> -->
		<div id="chart5" style="float: left; min-width: 150px; width: 49%; min-height: 200px;"></div>
		<div id="chart4" style="float: right; min-width: 150px; width: 49%; min-height: 100px;"></div>
		<div id="chart6" style="float: right; min-width: 150px; width: 49%; min-height: 100px;"></div>
