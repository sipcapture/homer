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
// echo '<script type="text/javascript" src="js/jquery.flot.js"></script>';
} else { $included = 0; }


date_default_timezone_set(CFLOW_TIMEZONE);
$offset = STAT_OFFSET;
$xhours = STAT_RANGE;

?>

		<script type="text/javascript" src="js/jquery.flot.js"></script>
		<script type="text/javascript" src="js/jquery.flot.pie.js"></script>

		<div id="chart2" style="min-width: 380px; width: 99%; margin-left: 1px; float: center; height: 220px"></div>



<script type="text/javascript">

$ = jQuery;

jQuery(document).ready(function() {


<?php

$uri = "http://".$_SERVER['SERVER_NAME'].APILOC;
$request = $uri."api.php?task=statsua&limit=12";
$jsondata = file_get_contents($request);
$response = json_decode($jsondata, true);
//print_r( $response);
foreach($response as $entry){
        foreach($entry as $uas){
        $sipUA[] = '{ label: \''.$uas['useragent'].'\', data: '.$uas['count'].'}';
        }
}


?>


var uas1 = [ <?php echo join($sipUA, ', ');?> ];


$.plot($("#chart2"), uas1, 
{
        series: {
            pie: { 
                show: true
            }
        },
        legend: {
            show: false
        }
});

});



</script>		

