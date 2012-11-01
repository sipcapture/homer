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
$API = APILOC;
if ($API == 'APILOC') {
include('../../configuration.php');
echo '<script type="text/javascript" src="js/highstock.js"></script>';
}

/* fix intranet web */
if(!defined('APIURL')) define('APIURL', "http://".$_SERVER['SERVER_NAME']);

$uri = APIURL.APILOC;
$request = $uri."api.php?task=statsua";
$jsondata = file_get_contents($request);
$response = json_decode($jsondata, true);
//print_r( $response);
foreach($response as $entry){
        foreach($entry as $uas){
        $sipUA[] = '{ name: \''.$uas['useragent'].'\', y: '.$uas['count'].'}';
        }
}

?>



<script type="text/javascript">

jQuery(function() {


var chart1 = new Highcharts.Chart({

      chart: {

         renderTo: 'chart3',
	 backgroundColor: null

      },

      title: {

         text: ''

      },

      legend: {

        enabled: false

      },

      tooltip: {

         formatter: function() {

            if (this.series.name == 'User-Agent') {

		if (this.point.name == '') { this.point.name = 'Undefined'; }

              return '<b>'+ this.point.name +'</b>: '+ this.percentage +' %';


            } else {

              return '<b>' + this.y + ' </b> ' + this.series.name + '';


            }

         }

      },

      series: [{

        name: 'User-Agent',

        type: 'pie',

        data: [
	<?php echo join($sipUA, ', ');?> ],

        dataLabels: {

          enabled: true,

          distance: 10, 

          formatter: function() {

            if (this.point.name == '') {

              return 'Undefined';

            } else {

              return this.point.name.substring(0,14);

            }

          }

        },
  	
//	center: [180, 80],
//        size: 60

      }


	]


   }); 
});


  


</script>		

		<!-- <script type="text/javascript" src="../stats/js/modules/exporting.js"></script> -->
		<div id="chart3" style="min-width: 380px; min-height: 200px;"></div>
