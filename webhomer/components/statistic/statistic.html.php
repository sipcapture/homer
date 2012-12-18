<?php
/*
 * HOMER Web Interface
 * Homer's homer.html.php
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

defined( '_HOMEREXEC' ) or die( 'Restricted access' );

class HTML_Statistic {

        static function displayStats() {
?>
  <div id="columns">
        <center>
        <ul id="column1" class="column" style="width: 10%;">
        </ul>

        <ul id="column2" class="column" style="width: 80%;">
            <li class="widget color-yellow" id="widget2">
                <div class="widget-head">                    
                  <select name="rangeStats" id="rangeStatsId" onchange="$('#stats0').load('modules/Graph/index_flot.php?range='+this.value); $('#stats1').load('modules/QoS/index_flot.php?range='+this.value); $('#stats2').load('modules/UAS/index_flot.php?range='+this.value);">
                    <option value="1">1 hour</option>
                    <option value="2">2 hours</option>
                    <option value="4">4 hours</option>
                    <option value="8">8 hours</option>
                    <option value="12">12 hours</option>
                    <option value="24">24 hours</option>
                    <option value="48">48 hours</option>
                    <option value="72">72 hours</option>
                    <option value="96">96 hours</option>
                 </select>
                 <h3>Capture Stats</h3>
                    
                </div>
                <div class="widget-content">


        <div id="Modules"></div><br>
<?php
        if ( CHARTER == 2 ) {
                $chart="flot";
                 echo "<script type=\"text/javascript\" src=\"js/jquery.flot.js\"></script>";
                 echo "<script type=\"text/javascript\" src=\"js/jquery.flot.pie.js\"></script>";
                 echo "<script type=\"text/javascript\" src=\"js/jquery.flot.threshold.js\"></script>";
        } else {
                $chart = "dyn";
        }

?>

	<script type="text/javascript">
        jQuery(document).ready( function($) {

<?php
	 if (isset($_SERVER['HTTP_USER_AGENT']) &&
                (strpos($_SERVER['HTTP_USER_AGENT'], 'MSIE') == true)) { exit; }

        // Scan Modules directory and display
        $submodules = array_filter(glob('modules/*'), 'is_dir');
        $modcount = 0;
        foreach( $submodules as $key => $value){
?>

                $('#Modules').append('<div id="stats<?php echo $modcount ?>" style="width:95%;height: auto;overflow: none;" />');
                $('#stats<?php echo $modcount ?>').load('<?php echo $value ?>/index_<?php echo $chart ?>.php');


<?php
        $modcount++;
        }

?>
                });
        </script>
	
</div>



<?php

  }

}

?>


