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

defined( '_HOMEREXEC' ) or die( 'Restricted access' );

class HTML_Statistic {

        function displayStats() {
?>
  <div id="columns">
        <center>
        <ul id="column1" class="column" style="width: 10%;">
        </ul>

        <ul id="column2" class="column" style="width: 80%;">
            <li class="widget color-yellow" id="widget2">
                <div class="widget-head">
                    <h3>Capture Stats ( <?php echo STAT_RANGE; ?>H )</h3>
                </div>
                <div class="widget-content">


        <div id="Modules"></div><br>

	<script type="text/javascript">
        jQuery(document).ready( function($) {

<?php

        // Scan Modules directory and display
        $submodules = array_filter(glob('modules/*'), 'is_dir');
        $modcount = 0;
        foreach( $submodules as $key => $value){
?>

                $('#Modules').append('<div id="stats<?php echo $modcount ?>" style="width:95%;height: auto;overflow: auto;" />');
                $('#stats<?php echo $modcount ?>').load('<?php echo $value ?>/index_dyn.php');


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


