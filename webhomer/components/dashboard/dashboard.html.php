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
		jQuery(document).ready( function($) {
                                $('.timepicker1').timeEntry({show24Hours: true, showSeconds: true, spinnerSize: [10, 10, 0], spinnerImage: ''});
                                $('.timepicker2').timeEntry({show24Hours: true, showSeconds: true, spinnerSize: [10, 10, 0], spinnerImage: ''});
                                // Date Picker
                                $('#from_date').datepicker({ dateFormat: 'dd-mm-yy' });
                                $('#to_date').datepicker({dateFormat: 'dd-mm-yy'});                                                                                                

				iNettuts.init();
		});
	</script>	
                                                         
<!-- admin mod start -->
  <div id="columns"  style="margin: 1px 1px 0 1px;">
	<center>

        <ul id="column1" class="column" style="width: 9%;">
		<br>

        </ul>

        <!-- <ul id="column2" class="column" style="margin: 0 0 0 0; min-height: 0px; height: 0px; width: 80%" > -->
        <ul id="column2" class="column">

<!-- start db tools -->
<?php

	}
	
	
	function displayStatsForm($search, $nodes) {	
?>	


<!-- column2 start -->
      
	<!-- about widget -->
                

	<li class="widget color-white" id="widget-search-noclose">
                <div class="widget-head"><h3>Quick Search - Time & Date</h3></div>
                <div class="widget-content"  style="max-height:250px;" >
                <br>
                <br>
                <table width="100%" style="min-width: 90px; width: 100%; margin-left: 1px; float: left; height: 220px;">
                <tr>
                   <td width="50%" valign="top">
                        <table width="100%" style="border: 1px solid #cfcfcf; border-radius: 5px; -moz-border-radius: 5px; padding: 5px;">

                                <tr>
                                        <td width="180" class="tablerow_large">
                                                <label for="from_user" title="Userpart in From URI">From User</label>
                                        </td>
                                        <td class="tableinputs_auto" >
                                                <input type="text" name="from_user" id="from_user" class="textfieldstyle-in" size="20" placeholder="A-Number" value="<?php if(isset($search['from_user'])) echo $search['from_user']; ?>" <?php if(WEBKIT_SPEECH) echo "x-webkit-speech onwebkitspeechchange='checkAnswer(this.id, this.value);'";?> />
                                        </td>
                                </tr>
	                        <tr>
                                        <td width="180" class="tablerow_large">
                                                <label for="ruri_user" title="B-Number in Request URI user part">RURI User</label>
                                        </td>
                                        <td class="tableinputs_auto" >
                                                <input type="text" name="ruri_user" id="ruri_user" class="textfieldstyle-in" size="20" placeholder="B-Number" value="<?php if(isset($search['ruri_user'])) echo $search['ruri_user']; ?>" <?php if(WEBKIT_SPEECH) echo "x-webkit-speech onwebkitspeechchange='checkAnswer(this.id, this.value);'";?> />
                                        </td>
                                </tr>                   
                                <tr>
                                        <td width="180" class="tablerow_large">
                                                <label for="to_user" title="Userpart in To URI">To User</label>
                                        </td>
                                        <td class="tableinputs_auto" >
                                                <input type="text" name="to_user" id="to_user" class="textfieldstyle-in" size="20" placeholder="B-Number (alt)" value="<?php if(isset($search['to_user'])) echo $search['to_user']; ?>" <?php if(WEBKIT_SPEECH) echo "x-webkit-speech onwebkitspeechchange='checkAnswer(this.id, this.value);'";?> />
                                        </td>
                                </tr>
                                <tr>
                                        <td width="180" class="tablerow_large">
                                                <label for="callid" title="Callid" onClick="document.homer.callid.value='';" >Call-ID</label>
                                        </td>
                                        <td class="tableinputs_auto" >
                                                <input type="text" name="callid" id="callid" placeholder="Call-ID" class="textfieldstyle-in" size="20" value="<?php if(isset($search['callid'])) echo $search['callid']; ?>" />
                                        </td>
                                </tr>
                                <tr>
				        <td width="180" class="tablerow_two"><label for="location" title="Homer DB Node">DB</label></td>
					<td class="tableinputs_auto">
					<?php
                                                if(isset($search['location']) && count($search['location'])) $locarray = $search['location'];
                                                else {
                                                            if(defined('DEFAULTDBNODE')) $locarray[DEFAULTDBNODE]=DEFAULTDBNODE;
                                                            else $locarray = array();
                                                }
                                                foreach ($nodes as $key=>$value) {
                                                ?>								
						    <input type="checkbox" class="checkboxstyle2" name="location[]" id="location" value="<?php echo $key?>" <?php if(in_array($key,$locarray)||$key==1) echo "checked"; ?>><?php echo $value;?>
                                                <?php
                                                }
                                        ?>
					</td>
                                </tr>

                                <tr>
	        		  <td style="padding-left:25%;padding-top:20px;" colspan="2" valign="bottom">
	        		    <input type="button" size="40" value="Quick Search" onClick="makeSearch();" class="ui-button ui-widget ui-state-default ui-corner-all">
                                  </td>
                                </tr>                             

	        	</table>
                    </td>
		    <td width="50%" valign="top">
            		<table width="100%" style="border: 1px solid #cfcfcf; border-radius: 5px; -moz-border-radius: 5px; padding: 5px;" >
                        <?php
                            $ft = date("H:i:s", strtotime("-1 hour"));
                            $tt = date("H:i:s");
        		?>
	        	    <tr>
        	        	    <td width="120" class="tablerow_two" VALIGN="top"><label for="date" title="Start">Start:</label></td>
	                	    <td class="tableinputs">
	                	        <input size="8" type="text" id="from_date"  class="textfieldstyle2" name="from_date" value="<?php echo date("d-m-Y");?> ">
                                        <input type="text" name="from_time" id="from_time" class="textfieldstyle2 timepicker1" size="6" value="<?php echo $ft;?>" />
                                    </td>
                            </tr>
                            <tr>
                                    <td width="120" class="tablerow_two" VALIGN="top"><label for="time" title="End">End:</label></td>
                                    <td class="tableinputs">
                                        <input size="8" type="text" id="to_date"  class="textfieldstyle2" name="to_date" value="<?php echo date("d-m-Y");?> ">
                                        <input type="text" name="to_time" id="to_time" class="textfieldstyle2 timepicker1" size="6" value="<?php echo $tt; ?>" />
                                    </td>
                            </tr> 

                            <tr>
				<td> 
				   <br>
                                </td>
                            </tr>  

                            <tr>
                                    <td width="150" class="tablerow_two">
				    </td>
                                    <td class="tableinputs">
				            <label for="time" title="Refresh">Autorefresh:</label>
                                            <select name="refresh" id="refresh" class="ui-select ui-widget ui-state-default ui-corner-all" >
                                                    <option value="0" selected>0</option>
                                                    <option value="10" >10</option>
                                                    <option value="15" >15</option>
                                                    <option value="30" >30</option>
                                                    <option value="60" >60</option>
                                                    <option value="120" >120</option>
                                                    <option value="300" >300</option>
                                            </select>
                                    </td>
                            </tr> 

                            <tr>
				<td style="padding-left:25%;padding-top:40px;" colspan="2" valign="bottom">
                                	<input type="button" size="40" value="Update Stats" onClick="showStats('all');" class="ui-button ui-widget ui-state-default ui-corner-all">
                                </td>
                            </tr>  


                        </table>
                    </td>
                  </tr></table>
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
<?php
	}
	
	
	function closeFirstColumn() {
?>	

        <!-- system/stats tab -->

        </ul><ul id="column3" class="column">

<?php	
        }

	function displayAlarms() {
	
?>	
  
	<li  class="widget color-red" id="widget-alarm">
		<div class="widget-head"><h3>Alarms</h3></div>
                <div class="widget-content">
                <br>
                <?php 
                        include("modules/shortAlarm/index_data.php");                         
                ?>
                <br>
        	</div>
	</li>

<?php
	}	


	


	function displayQoS() {
	
?>	

    

	<li  class="widget color-green" id="widget-prefs" style="background:#f2bc00;">
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
  
	<li  class="widget color-yellow" id="widget-prefs">
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
  
	<li  class="widget color-yellow" id="widget-prefs">
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
