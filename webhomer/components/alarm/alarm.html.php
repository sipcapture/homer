<?php 
/*
 * HOMER Web Interface
 * Homer's admin.html.php
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
?>

<?php

defined( '_HOMEREXEC' ) or die( 'Restricted access' );

class HTML_Stats {

	function showStatisticMain() {

?>
	<style type="text/css" title="currentStyle">
			@import "styles/demo_page.css";
                        @import "styles/demo_table.css";
	</style>
        <script src="js/jquery.timeentry.js" type="text/javascript"></script>
        <script src="js/jquery.mousewheel.js" type="text/javascript"></script>                        
	<script type="text/javascript" src="js/cookie.jquery.js"></script>
	<script type="text/javascript" src="js/inettuts3.js"></script> 
        <script type="text/javascript" src="js/jquery.Dialog.js"></script>

	<script type="text/javascript">
		jQuery(document).ready( function($) {
                                $('.timepicker1').timeEntry({show24Hours: true, showSeconds: true});
                                $('.timepicker2').timeEntry({show24Hours: true, showSeconds: true});
                                // Date Picker
                                $('#from_date').datepicker({ dateFormat: 'dd-mm-yy' });
                                $('#to_date').datepicker({dateFormat: 'dd-mm-yy'});                                                                                                
                                
		});
	</script>	
	
	
                                                         
<!-- admin mod start -->
  <div id="columns"  style="margin: 1px 1px 0 1px;">
	<center>

        <ul id="column1" class="column" style="width: 9%;">
		<br>

        </ul>

        <ul id="column2" class="column" style="margin: 0 0 0 0; min-height: 0px; height: 0px; width: 80%" >

<!-- start db tools -->
<?php

	}
	
	

	function displayAlarm() {	
?>	


<!-- column2 start -->
      
	<!-- about widget -->



	<li class="widget color-red" id="widget-about">
                <div class="widget-head"><h3>HOMER Alarms</h3></div>
                <div class="widget-content" style="height:auto;">
                <br>
                <br>
                        <?php
                            $ft = date("H:i:s", strtotime("-1 hour"));
                            $tt = date("H:i:s");
                        ?>
                        <label for="date" title="From date">From Date:</label>
                        <input size="10" type="text" id="from_date"  class="textfieldstyle2" name="from_date" value="<?php echo date("d-m-Y");?> ">
                                            &nbsp;-&nbsp;  
                        <input type="text" name="from_time" id="from_time" class="textfieldstyle2 timepicker1" size="8" value="<?php echo $ft;?>" />

                        <label for="time" title="To Date" style="margin-left: 20px;" >To Date:</label>
                                        <input size="10" type="text" id="to_date"  class="textfieldstyle2" name="to_date" value="<?php echo date("d-m-Y");?> ">
                                            &nbsp;-&nbsp;
                                        <input type="text" name="to_time" id="to_time" class="textfieldstyle2 timepicker2" size="8" value="<?php echo $tt; ?>" />
                        <label for="time" title="NewAlarm" style="margin-left: 20px;">Type:</label>
                                            <select name="alarmtype" id="alarmtype" class="ui-select ui-widget ui-state-default ui-corner-all" >
                                                    <option value="2" selected>All</option>
                                                    <option value="1" >New</option>
                                                    <option value="0" >Old</option>
                                            </select>
                        <label for="time" title="Refresh" style="margin-left: 20px;">Autorefresh:</label>
                                            <select name="refresh" id="refresh" class="ui-select ui-widget ui-state-default ui-corner-all" >
                                                    <option value="0" selected>0</option>
                                                    <option value="10" >10</option>
                                                    <option value="15" >15</option>
                                                    <option value="30" >30</option>
                                                    <option value="60" >60</option>
                                                    <option value="120" >120</option>
                                                    <option value="300" >300</option>
                                            </select>                                     
		         <br><br>
                          <input type="button" size="50" style="margin-left: 20px;" value="Show Alarms" onClick="showAlarms('all');" class="ui-button ui-widget ui-state-default ui-corner-all">
                <?php
                        include("modules/alarm/index_data.php");
                ?>
                <br>
 		</div>
	</li>
<!-- system/stats tab -->

<?php
	}
	
	function closeUlColumn() {
	
	    echo "</ul>";
	
	}
}

?>
