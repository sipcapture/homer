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
 * as published by the Free Software Foundation; either version 3
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

if (!defined('APILOC')) {
$included = 1;
include('../../configuration.php');
} else { $included = 0; }

/* fix intranet web */
if(!defined('APIURL')) define('APIURL', "http://".$_SERVER['SERVER_NAME']);

date_default_timezone_set(CFLOW_TIMEZONE);
$offset = STAT_OFFSET;
$xhours = STAT_RANGE;

?>

		<div id="chart2" style="min-width: 380px; width: 99%; margin-left: 1px; float: center; height: 220px"></div>



<script type="text/javascript">

$ = jQuery;

$(document).ready(function() {


<?php

$uri = APIURL.APILOC;
$request = $uri."api.php?task=statsua&limit=11";
$jsondata = file_get_contents($request);
$response = json_decode($jsondata, true);
//print_r( $response);
foreach($response as $entry){
        foreach($entry as $uas){
	  if ($uas['useragent'] && $uas['count'] && is_numeric($uas['count']) ) {
            $sipUA[] = '{ label: \''.$uas['useragent'].'\', data: '.$uas['count'].'}';
	  }
        }
}


?>


var uas1 = [ <?php if (!empty($sipUA)) { echo join($sipUA, ', '); } ?> ];

$.plot($("#chart2"), uas1, 
{
        series: {
            pie: { 
                show: true
            }
        },
        legend: {
            show: true,
	    labelFormatter: function(label, series) {
   		 // series is the series object for the label
   		 return ' ' + label.slice(0,30) + ' ('+Math.round(series.percent)+'%)';
  		}
        }
});

});



</script>		

