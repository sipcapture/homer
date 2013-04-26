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

        <script src="js/jquery.timeentry.js" type="text/javascript"></script>
        <script src="js/jquery.mousewheel.js" type="text/javascript"></script>                        
	<script type="text/javascript" src="js/cookie.jquery.js"></script>
	<script type="text/javascript" src="js/inettuts3.js"></script> 
        <script type="text/javascript" src="js/jquery.Dialog.js"></script>
        <script type="text/javascript" src="js/jquery.flot.js"></script>
        <script type="text/javascript" src="js/jquery.flot.pie.js"></script>
        <script type="text/javascript" src="js/jquery.flot.threshold.js"></script>

	<script type="text/javascript">
        	function loadAlarmData() {return;}
        	
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
	
	
	function displayStatsForm($search, $nodes) {	
?>	


<!-- column2 start -->
      
	<!-- about widget -->


	<li class="widget color-white" id="widget-form">
                <div class="widget-head"><h3>Statistic Range - Time & Date</h3></div>
                <div class="widget-content">
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
                                     <input type="button" size="30" style="margin-left: 20px;" value="Show Stat" onClick="showStats('all');" class="ui-button ui-widget ui-state-default ui-corner-all">
 		</div>
	</li>
<!-- system/stats tab -->

<?php
	}
	

	function displayChartsLine() {	
?>	


<!-- column2 start -->
      
	<!-- about widget -->



	<li class="widget color-blue" id="widget-about">
                <div class="widget-head"><h3>Charts</h3>
                AuthFail: <input type="checkbox" class="checkboxdstyle2" name="chart_authfail" id="chart_authfail" value="1" onClick="javascript:showStats('dataCharts');" checked>
                Packets: <input type="checkbox" class="checkboxdstyle2" name="chart_packets" id="chart_packets" value="1" onClick="javascript:showStats('dataCharts');" checked>
                Calls: <input type="checkbox" class="checkboxdstyle2" name="chart_calls" id="chart_calls" value="1" onClick="javascript:showStats('dataCharts');" checked>
                ASR: <input type="checkbox" class="checkboxdstyle2" name="chart_asr" id="chart_asr" value="1" onClick="javascript:showStats('dataCharts');" checked>
                        
                </div>
                <div class="widget-content">
                <br>
                <?php include("modules/Graph/index_data.php"); ?>
                <br>
 		</div>
	</li>
<!-- system/stats tab -->

<?php
	}
	


	function displayQoS() {
	
?>	

    

	<li  class="widget color-red" id="widget-prefs" style="background:#f2bc00;">
		<div class="widget-head"><h3>QoS</h3></div>
                <div class="widget-content">
                <br>
                <?php 
                        include("modules/QoS/index_data.php");                
                ?>
                <br>
        	</div>
	</li>

<?php
	}
	
	function displayUAS() {
	
?>	
	<li  class="widget color-green" id="widget-prefs">
		<div class="widget-head"><h3>UAS</h3></div>
                <div class="widget-content">
                <br>
                <?php 
                        include("modules/UAS/index_data.php"); 
                        
                ?>
                <br>
        	</div>
	</li>

<?php
	}	
	

	function displayIP() {
	
?>	
	<li  class="widget color-green" id="widget-prefs">
		<div class="widget-head"><h3>IP</h3></div>
                <div class="widget-content">
                <br>
                <?php 
                        include("modules/IP/index_data.php"); 
                        
                ?>
                <br>
        	</div>
	</li>

<?php
	}	
	


	function closeUlColumn() {
	
	    echo "</ul>";
	
	}
}

?>
