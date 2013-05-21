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

class HTML_search {

	static function displaySearchForm ($search, $nodes) {

?>	
    	        
		<script src="js/jquery.timeentry.js" type="text/javascript"></script>
		<script src="js/jquery.mousewheel.js" type="text/javascript"></script>
  	    	<script type="text/javascript" src="js/cookie.jquery.js"></script>
		<script type="text/javascript" src="js/inettuts3.js?<?php echo time(); ?>"></script>
	
	         <script type="text/javascript">
	         
	         	$.noConflict();	         
	
	         	jQuery(document).ready( function($) {


		                $('.timepicker1').timeEntry({show24Hours: true, showSeconds: true}); 
		                $('.timepicker2').timeEntry({show24Hours: true, showSeconds: true}); 

				$('#from_user').autocomplete({
                                source: "utils.php?task=livesearch&field=from_user&from_date="+$('#from_date').val()+"&to_date="+$('#to_date').val()+"&from_time="+$('#from_time').val()+"&to_time="+$('#to_time').val(),
                                minLength: 3,
                                select: function(event, ui) {
                                        $('#from_user').val(ui.item.from_user);
                                }

	                        });
	                        
        	                $('#to_user').autocomplete({
                                source: "utils.php?task=livesearch&field=to_user&from_date="+$('#from_date').val()+"&to_date="+$('#to_date').val()+"&from_time="+$('#from_time').val()+"&to_time="+$('#to_time').val(),
                                minLength: 3,
                                select: function(event, ui) {
                                        $('#to_user').val(ui.item.to_user);
                                }
                                });                                

				// Date Picker
				$('#from_date').datepicker({ dateFormat: 'dd-mm-yy' });
				$('#to_date').datepicker({dateFormat: 'dd-mm-yy'});

				iNettuts.init();

				 // Simple Search Filters - NOT FOR RELEASE
                                    $('#filters_a').change(function() {
                                        // var x = $(this).find('option:selected').text();
                                        var x = $(this).val();
                                                if (x == 'INVITE') {
                                                        //$('#method').val('200');
                                                        $('#cseq').val('%INVITE');
                                                }

                                                if (x == 'REGISTER') {
                                                        //$('#method').val('200');
                                                        $('#cseq').val('%REGISTER');
                                                        //$('#filters_b').append(new Option('', '', true, true));
                                                        //$('#filters_b').append(new Option('OK', '200', true, true));
                                                        //$('#filters_b').append(new Option('KO', '401', true, true));

                                                }
						if (x == '') {
                                                        $('#method').val('');
                                                        $('#cseq').val('');
                                                }

                                 });

			});


	        
		</script>	                	    




<br>
	<!-- <div class="wrapper"> -->
	<!-- <div id="results"> -->

	<!-- extra code tables -->
	 <div id="columns">
        
        <ul id="column1" class="column">
			
            <li class="widget color-blue" id="widget-header">  
                <div class="widget-head">
                    <h3>Header Details</h3>
                </div>
                <div class="widget-content">
                     <table class="bodystyle" cellspacing="1" height="135">
				<tr>
					<td width="150" class="tablerow_two">
						<label for="ruri" title="RURI">RURI</label>
					</td>
					<td class="tableinputs">
						<input type="text" name="ruri" id="ruri" class="textfieldstyle-in" size="40" value="<?php if(isset($search['ruri'])) echo $search['ruri']; ?>" />
					</td>
				</tr>
				<tr>
					<td width="150" class="tablerow_two">
						<label for="via_1" title="VIA">VIA 1</label>
					</td>
					<td class="tableinputs">
						<input type="text" name="via_1" id="via_1" class="textfieldstyle-in" size="40" value="<?php if(isset($search['via_1'])) echo $search['via_1']; ?>" />
					</td>
				</tr>	
				<tr>
					<td width="150" class="tablerow_two">
						<label for="diversion" title="Diversion">Diversion</label>
					</td>
					<td class="tableinputs">
						<input type="text" name="diversion" id="diversion" class="textfieldstyle-in" size="40" value="<?php if(isset($search['diversion'])) echo $search['diversion']; ?>" />
					</td>
				</tr>	
				<tr>
					<td width="150" class="tablerow_two">
						<label for="cseq" title="Cseq">Cseq</label>
					</td>
					<td class="tableinputs">
						<input type="text" name="cseq" id="cseq" class="textfieldstyle-in" size="40" value="<?php if(isset($search['cseq'])) echo $search['cseq']; ?>" />
					</td>
				</tr>	
				<tr>
					<td width="150" class="tablerow_two">
						<label for="reason" title="Reason">Reason</label>
					</td>
					<td class="tableinputs">
						<input type="text" name="reason" id="reason" class="textfieldstyle-in" size="40" value="<?php if(isset($search['reason'])) echo $search['reason']; ?>" />
					</td>
				</tr>	
				<tr>
					<td width="150" class="tablerow_two">
						<label for="reason" title="Reason">Method</label>
					</td>
					<td class="tableinputs">
						<input type="text" name="method" id="method" class="textfieldstyle-in" size="40" value="<?php if(isset($search['method'])) echo $search['method']; ?>" />
					</td>
				</tr>	
<!--				<tr>
					<td width="150" class="tablerow_two">
						<label for="content-type" title="Content-Type">Content-Type</label>
					</td>
					<td class="tableinputs">
						<input type="text" name="content_type" id="content_type" class="textfieldstyle-in" size="40" value="<?php if(isset($search['content_type'])) echo $search['content_type']; ?>" />
					</td>
				</tr> 
				<tr>
					<td width="150" class="tablerow_two">
						<label for="authorization" title="Authorization">Authorization</label>
					</td>
					<td class="tableinputs">
						<input type="text" name="authorization" id="authorization" class="textfieldstyle-in" size="40" value="<?php if(isset($search['authorization'])) echo $search['authorization']; ?>" />
					</td>
				</tr>	
-->
				<tr>
					<td width="150" class="tablerow_two">
						<label for="user_agent" title="User-Agent">User-Agent</label>
					</td>
					<td class="tableinputs">
						<input type="text" name="user_agent" id="user_agent" class="textfieldstyle-in" size="40" value="<?php if(isset($search['user_agent'])) echo $search['user_agent']; ?>" />
					</td>
				</tr>															
				<tr>
					<td width="150" class="tablerow_two">
						<label for="msg" title="Msg">Message</label>
					</td>
					<td class="tableinputs">
						<input type="text" name="msg" id="msg" class="textfieldstyle-in" size="40" value="<?php if(isset($search['msg'])) echo $search['msg']; ?>" />
					</td>
				</tr>																			
			</table><br>
                </div>
            </li>
	 <li class="widget color-blue" id="widget-network">
                <div class="widget-head">
                    <h3>Network Filter</h3>
                </div>
                <div class="widget-content">

					<table class="bodystyle" cellspacing="1" height="135">
				<tr>
					<td width="150" class="tablerow_two">
						<label for="source_ip" title="Source IP">Source IP</label>
					</td>
					<td class="tableinputs" style="width:50%;" >
						<input type="text" name="source_ip" id="source_ip" class="textfieldstyle-in" size="40" value="<?php if(isset($search['source_ip'])) echo $search['source_ip']; ?>" />
					</td>
					<td >
						<label for="source_port" title="Source PORT">Port</label>
					</td>
					<td class="tableinputs" style="width:20%;">
						<input type="text" name="source_port" id="source_port" class="textfieldstyle-in" size="5" value="<?php if(isset($search['source_port'])) echo $search['source_port']; ?>" />
					</td>
				</tr>
				<tr>
					<td width="150" class="tablerow_two">
						<label for="destination_ip" title="Destination IP">Destination IP</label>
					</td>
					<td class="tableinputs" style="width:50%;" >
						<input type="text" name="destination_ip" id="destination_ip" class="textfieldstyle-in" size="40" value="<?php if(isset($search['destination_ip'])) echo $search['destination_ip']; ?>" />
					</td>
					<td >
						<label for="destination port" title="Destination PORT">Port</label>
					</td>
					<td class="tableinputs" style="width:10%;">
						<input type="text" name="destination_port" id="destination_port" class="textfieldstyle-in" size="5" value="<?php if(isset($search['destination_port'])) echo $search['destination_port']; ?>" />
					</td>
				</tr>	
				<tr>
					<td width="150" class="tablerow_two">
						<label for="contact_ip" title="Contact IP">Contact IP</label>
					</td>
					<td class="tableinputs" style="width:50%;" >
						<input type="text" name="contact_ip" id="contact_ip" class="textfieldstyle-in" size="40" value="<?php if(isset($search['contact_ip'])) echo $search['contact_ip']; ?>" />
					</td>
					<td >
						<label for="contact port" title="Contact PORT">Port</label>
					</td>
					<td class="tableinputs" style="width:10%;">
						<input type="text" name="contact_port" id="contact_port" class="textfieldstyle-in" size="5" value="<?php if(isset($search['contact_port'])) echo $search['contact_port']; ?>" />
					</td>
				</tr>
				<tr>
					<td width="150" class="tablerow_two">
						<label for="originator_ip" title="Originator IP">Originator IP</label>
					</td>
					<td class="tableinputs" style="width:50%;">
						<input type="text" name="originator_ip" id="originator_ip" class="textfieldstyle-in" size="40" value="<?php if(isset($search['originator_ip'])) echo $search['originator_ip']; ?>" />
					</td>
					<td >
						<label for="originator port" title="Originator PORT">Port</label>
					</td>
					<td class="tableinputs" style="width:10%;">
						<input type="text" name="originator_port" id="originator_port" class="textfieldstyle-in" size="5" value="<?php if(isset($search['originator_port'])) echo $search['originator_port']; ?>" />
					</td>
				</tr>				
				<tr>
					<td width="20%" class="paramlist_key"><label for="proto" title=".">Proto</label></td>
						<td class="tablerow_two">
							<select name="proto" id="proto" class="ui-select ui-widget ui-state-default ui-corner-all" >
							        <option value="1" >TCP</option>
								<option value="2" selected="selected">UDP</option>
								<option value="3" >TLS/SSL</option>
								<option value="4" >SCTP</option>
							</select>
					</td>
				</tr>
				<tr>
					<td width="20%" class="paramlist_key"><label for="family" title=".">Family</label></td>
						<td class="tablerow_two">
							<select name="family" id="family" class="ui-select ui-widget ui-state-default ui-corner-all" >
								<option value="1" selected="selected">IPv4</option>
								<option value="2" >IPv6</option>
							</select>
					</td>
				</tr>
			</table>
			<br>

		</div>
	</li>

<!--
	 <li class="widget color-green" id="alarms-widget">
                <div class="widget-head">
                    <h3>Server Health</h3>
                </div>
                <div class="widget-content">


		<br>
                <table class="bodystyle" cellspacing="1" height="25">
                                <tr>
                                        <td width="150" class="tablerow_two">
                                                Monitored Services:
                                        </td>
                                        <td>

		<?php 

		$alarm_status = 0;

		$kamailio = exec('ps ax | grep -v grep | grep kamailio.pid | wc -l'); 
		if ($kamailio > 0 ) { 
			echo "KAMAILIO OK (".$kamailio.")";  
		} 
		else {  echo "KAMAILIO DOWN"; $alarm_status =  $alarm_status + 1; } 
		
		echo ", ";
		
		$mysql = exec('ps ax | grep -v grep | grep "mysqld " | wc -l'); 
		if ($mysql > 0 ) { 
			echo "MYSQL OK (".$mysql.")";  
		}
		else {  echo "MYSQL DOWN"; $alarm_status =  $alarm_status + 1; } 


	//	echo "</li><li>";
	//
	//	$fake = exec('ps ax | grep -v grep | grep fakeservice | wc -l');
        //      if ($fake > 0 ) {
        //                echo "FAKE OK (".$fake.")"; 
        //        }
        //        else {  echo "FAKE DOWN"; $alarm_status =  $alarm_status + 1; }


		if ($alarm_status != 0) { 
	?>
		<script type="text/javascript"> 
		 JQuery('#alarms-widget').removeClass("color-green").addClass("color-red");
		</script> 
	<?php 
		} else {  
	?>
		 <script type="text/javascript">

                        jQuery('#alarms-widget').removeClass("color-red").addClass("color-green");

                </script>
        <?php 
		}  
	?>

		</td></tr></table>
		<br>


 
              </div>
            </li>
-->

        </ul>
<!-- END OF BASE COLUMN -->

        <ul id="column2" class="column">
			
            <li class="widget color-blue" id="Widget-user">
                <div class="widget-head">
                    <h3>User Details</h3>
                </div>
                <div class="widget-content"><br>
                   <table class="bodystyle" cellspacing="1"  height="150">
				<tr>
					<td width="150" class="tablerow_two">
						<label for="ruri_user" title="B-Number in Request URI user part">RURI User *</label>
					</td>
					<td class="tableinputs">
						<input type="text" name="ruri_user" id="ruri_user" class="textfieldstyle-in" size="40" value="<?php if(isset($search['ruri_user'])) echo $search['ruri_user']; ?>" <?php if(WEBKIT_SPEECH) echo "x-webkit-speech onwebkitspeechchange='checkAnswer(this.id, this.value);'";?> />
					</td>
				</tr>                   
				<tr>
					<td width="150" class="tablerow_two">
						<label for="from_user" title="Userpart in From URI">From User</label>
					</td>
					<td class="tableinputs">
						<input type="text" name="from_user" id="from_user" class="textfieldstyle-in" size="40" value="<?php if(isset($search['from_user'])) echo $search['from_user']; ?>" <?php if(WEBKIT_SPEECH) echo "x-webkit-speech onwebkitspeechchange='checkAnswer(this.id, this.value);'";?> />
					</td>
				</tr>
				<tr>
					<td width="150" class="tablerow_two">
						<label for="to_user" title="Userpart in To URI">To User</label>
					</td>
					<td class="tableinputs">
						<input type="text" name="to_user" id="to_user" class="textfieldstyle-in" size="40" value="<?php if(isset($search['to_user'])) echo $search['to_user']; ?>" <?php if(WEBKIT_SPEECH) echo "x-webkit-speech onwebkitspeechchange='checkAnswer(this.id, this.value);'";?> />
					</td>
				</tr>
				<tr>
					<td width="150" class="tablerow_two">
						<label for="pid_user" title="P-Asserted and P-Preffered">PID User</label>
					</td>
					<td class="tableinputs">
						<input type="text" name="pid_user" id="pid_user" class="textfieldstyle-in" size="40" value="<?php if(isset($search['pid_user'])) echo $search['pid_user']; ?>" />
					</td>
				</tr>
				<tr>
					<td width="150" class="tablerow_two">
						<label for="contact_user" title="Contact header"">Contact User</label>
					</td>
					<td class="tableinputs">
						<input type="text" name="contact_user" id="contact_user" class="textfieldstyle-in" size="40" value="<?php if(isset($search['contact_user'])) echo $search['contact_user']; ?>" />
					</td>
				</tr>
				<tr>
					<td width="150" class="tablerow_two">
						<label for="auth_user" title="Proxy-Auth, WWW-Auth">Auth User</label>
					</td>
					<td class="tableinputs"	>
						<input type="text" name="auth_user" id="auth_user" class="textfieldstyle-in" size="40" value="<?php if(isset($search['auth_user'])) echo $search['auth_user']; ?>" />
					</td>
				</tr>
			</table><br>
               
				</div>
            </li>
			
            <li class="widget color-blue" id="widget-call">  
                <div class="widget-head">
                    <h3>Call Details</h3>
                </div>
                <div class="widget-content">
                   <table class="bodystyle" cellspacing="1"  height="50">
			 	<tr>
					<td width="150" class="tablerow_two">
						<label for="callid" title="Callid" onClick="document.homer.callid.value='';" >Call-ID</label>
					</td>
					<td class="tableinputs">
						<input type="text" name="callid" id="callid" class="textfieldstyle-in" size="40" value="<?php if(isset($search['callid'])) echo $search['callid']; ?>" />
					</td>
                                </tr>
                                <tr>					
					<td width="150" class="tablerow_two">
                                                <label for="callid_aleg" title="Search Call-ID as bridged call">B2B Call-ID</label>
                                        </td>
                                        <td>
                                                <input type="checkbox" name="b2b" id="b2b" class="checkboxdstyle2" value="1" <?php if(isset($search['b2b']) && $search['b2b'] == 1) echo "checked"; ?>/>
					</td>

				</tr>
			</table>
                   </div>
            </li>

        </ul>
        
        <ul id="column3" class="column">
			
			
            <li class="widget color-orange" id="widget-time">  
                <div class="widget-head">
                    <h3>Time & Date</h3>
                </div>
                <div class="widget-content">
                    <table class="bodystyle" cellspacing="1" height="150">						
							<tr>
							
								<td width="20%" class="paramlist_key"><label for="location" title="Homer DB Node">DB Location</label></td>
								<td class="tablerow_two">
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
                           <td width="20%" class="paramlist_key"><label for="location" title="Homer Capture Node">Capture node</label></td>
                           <td class="tablerow_two">
                           <input name="node" id="node" size="6" value="<?php if(isset($search['node'])) echo $search['node'];?>" class="ui-select ui-widget ui-state-default ui-corner-all"  />
                                   <!--
                                    <select name="node" id="node" class="ui-select ui-widget ui-state-default ui-corner-all"  />
                                    <option value="101" <?php if(isset($search['node']) && (($search['node']) == "101" )) echo 'selected="selected"'; ?> >Berlin [101]</option>
                                    <option value="102" <?php if(isset($search['node']) && (($search['node']) == "102" )) echo 'selected="selected"'; ?> >Frankfurt [102]</option>
                                    <option value="103" <?php if(isset($search['node']) && (($search['node']) == "103" )) echo 'selected="selected"'; ?> >Cologne [103]</option>                                     
                                    </select>
                                  -->
                           </td>
             </tr>                    
							<?php
									$ft = date("H:i:s", strtotime("-1 hour"));
									$tt = date("H:i:s");
							
							
							?>
							<tr>
								<td width="20%" class="paramlist_key"><label for="date" title="Searching from this date">From Date</label></td>
								<td class="tablerow_two">
									<input size="8" type="text" id="from_date"  class="textfieldstyle2" name="from_date" value="<?php if(isset($search['from_date'])) echo $search['from_date']; else echo date("d-m-Y");?> ">
								&nbsp;-&nbsp;
								        <input type="text" name="from_time" id="from_time" class="textfieldstyle2 timepicker1" size="5" value="<?php if(isset($search['from_time'])) echo $search['from_time']; else echo $ft;?>" />
									
								</td>
							</tr>
							<tr>
								<td width="20%" class="paramlist_key"><label for="time" title="Searching up to this date">To Date</label></td>
								<td class="tablerow_two">
                                                                      <input size="8" type="text" id="to_date"  class="textfieldstyle2" name="to_date" value="<?php if(isset($search['to_date'])) echo $search['to_date']; else echo date("d-m-Y");?> ">
                                                                      &nbsp;-&nbsp;
								      <input type="text" name="to_time" id="to_time" class="textfieldstyle2 timepicker2" size="5" value="<?php if(isset($search['to_time'])) echo $search['to_time']; else echo $tt; ?>" />
								</td>
							</tr>
                                                        <!--
							<tr>
								<td width="20%" class="paramlist_key"><label for="maximum" title=".">Maximum records</label></td>
								<td class="tablerow_two">
									<input type="text" name="max_records" id="max_records" class="textfieldstyle2" size="4" value="<?php if(isset($search['max_records'])) echo $search['max_records']; else echo '100'; ?>" />
								</td>
							</tr>
							-->
                        				<tr>
	                	        			<td width="150" class="tablerow_two">
		        	        	        		<label for="unique" title="Unique packet">Uniq packets</label>
                	        				</td>
				                        	<td>
                        						<input type="checkbox" name="unique" id="unique" class="checkboxdstyle" value="1" <?php if(isset($search['unique']) && $search['unique'] == 1) echo "checked"; ?> />
			                        		</td>
                        				</tr>
                        				<tr>
	                	        			<td width="150" class="tablerow_two">
		        	        	        		<label for="unique" title="Unique packet">Logic OR</label>
                	        				</td>
				                        	<td>
                        						<input type="checkbox" name="logic_or" id="logic_or" class="checkboxdstyle" value="1" <?php if(isset($search['logic_or']) && $search['logic_or'] == 1) echo "checked"; ?> />
			                        		</td>
                        				</tr>
                                   <tr>
                                    <td width="150" class="tablerow_two">
                                       <label for="limit" title="Limit">Limit</label>
                                     </td>
                                    <td>
                                                <input name="limit" id="limit" value="<?php if(isset($search['limit'])) echo $search['limit']; else echo "100";?>" class="ui-select ui-widget ui-state-default ui-corner-all"  />
                                     </td>
                                  </tr>
						</table><br>	
                </div>
            </li>


            <li class="widget color-white" id="widget-search-noclose">  
                <div class="widget-head">
                    <h3>Search Submit</h3>
                </div>
                <div class="widget-content">
                   <table class="bodystyle" cellspacing="1"  height="50">
				<tr>
					<td>           </td>
					<td>
						<input type="submit" size="30" value="Search Homer" onClick="return check_form();" class="ui-button ui-widget ui-state-default ui-corner-all">
					</td>
					<td>
						<input type="button" size="30" value="Clear All" onClick="return iNettuts.killForm('homer');" class="ui-button ui-widget ui-state-default ui-corner-all">
					</td>
					  <td>&nbsp; &nbsp; &nbsp; &nbsp; </td>
                                         <td>
                                                <select name="filters_a" id="filters_a" value="" class="ui-select ui-widget ui-state-default ui-corner-all"  />
                                                  <option value="" ></option>

						<option value="INVITE" <?php if(isset($search['cseq']) && (($search['cseq']) == "%INVITE" )) echo 'selected="selected"'; ?> >Calls</option>
						<option value="REGISTER" <?php if(isset($search['cseq']) && (($search['cseq']) == "%REGISTER" )) echo 'selected="selected"'; ?> >Registrations</option>

                                                </select>

                                        </td>


				</tr>
			</table>
                </div>
            </li>
            
        </ul>
        
    </div>

	<!-- extra end -->			

	<!-- </div> -->


	<span class="note">
	<br />
	<!-- </div> -->

<!-- end search -->
																														
<?php
	}


	function displayResultSearch ($table,$ft,$tt,$search, $showColumns) {
	
		//Color Generator
		srand(floor(time() / (60*60*24)));
		$hex = array("00", "33", "66", "99", "CC", "FF");	 					
?>		
            <style type="text/css" title="currentStyle">
			@import "styles/demo_page.css";
                        @import "styles/demo_table.css";
                        @import "styles/demo_table_jui.css";
            </style>            

	<script type="text/javascript" language="javascript" src="js/jquery.dataTables.min.js"></script>        
        <script type="text/javascript" src="js/jquery.Dialog.js"></script>

	<script type="text/javascript">		
	 	$(document).mousemove(function(e){
                                        $('body').data('posx', e.pageX);
                                        $('body').data('posy', e.pageY);
                        });
	</script>


<div style="display: inline; float: left; font-weight: bold; solid #C0C0C0;padding: 5px;" id="deltacalc">
  &#916; <input type="text" name="Display" id="delta_value_1" align="right" class="textfieldstyle2"> -
  <input type="text" name="Display2" id="delta_value_2" align="right" class="textfieldstyle2"> =
  <input type="text" id="delta_result" name="Display3" align="right" size="10" class="textfieldstyle2"> &micro;
</div>
<div style="display: inline; float: right ; width: auto; font-weight: bold; padding: 5px;">
<i>Result</i>: [<?php echo date("d-m-Y H:i:s", strtotime($ft));?> - <?php echo date("d-m-Y H:i:s", strtotime($tt));?>].
<?php
                $ignore =  array('from_date', 'to_date', 'from_time', 'to_time');
                $mydata = array();
                foreach($search as $key=>$value) {

                        if($value == '' || in_array($key,$ignore)) continue;
                        if($key == "location") $mydata[]="$key=".implode($value,",");
                        else $mydata[]="$key=$value";
                }
                echo "<i>Params</i>: [ ".implode($mydata, "; ")." ]";
?>
</div>

<?php echo $table->render(); ?>

        <script type="text/javascript" language="javascript">
                
        function fnShowHide( iCol ) {
		/* Get the DataTables object again - this is not a recreation, just a get of the object */
		var oTable = $('#SIPTable').dataTable();	
		var bVis = oTable.fnSettings().aoColumns[iCol].bVisible;
		oTable.fnSetColumnVis( iCol, bVis ? false : true );
	}

        $.fn.dataTableExt.oApi.fnFilterClear  = function ( oSettings ) {
                /* Remove global filter */
                oSettings.oPreviousSearch.sSearch = "";
                if ( typeof oSettings.aanFeatures.f != 'undefined' ) {
                        var n = oSettings.aanFeatures.f;
                        for ( var i=0, iLen=n.length ; i<iLen ; i++ ) { $('input', n[i]).val( '' ); }
                }
                for ( var i=0, iLen=oSettings.aoPreSearchCols.length ; i<iLen ; i++ ) { oSettings.aoPreSearchCols[i].sSearch = ""; }
                oSettings.oApi._fnReDraw( oSettings );
        }
        
        $(document).ready( function () {
                $('#deltacalc').hide();
                
                var checkedStatus = false;
                $('#SIPTable th:eq(0)').click(function() {
                        if(checkedStatus == false) checkedStatus = true;
                        else checkedStatus = false;
                        $("#SIPTable tbody tr td:first-child input:checkbox").each(function() {
                                this.checked = checkedStatus;
                        });
                });
        } );
        
        </script>

<div id="dialog-form" title="Select fields to show">
	<p class="validateTips">Click on checkbox to hide/show.</p>

	<form>
	<fieldset>

	<table border="0" class="layout-grid" width="100%">
<?php
        $i=0;
        foreach($showColumns as $key=>$column) {
            if($i==0 || $i==4) { echo "<tr>"; $i=0; }
            print "<td><input type='checkbox' onClick='javascript:fnShowHide($key);' class='text ui-widget-content ui-corner-all' ";
            if($column["visible"]) echo "checked";
            print " /></td>";
            print "<td>".$column["title"]."</td>";
            if($i==3) echo "</tr>";
            $i++;
        }
        if($i < 4) echo "</tr>";
?>
        </table>	
	</fieldset>
	</form>
</div>


<?php
	}
	
	function displayMessage($rows) {
?>	
		<table border="0" cellspacing="2" cellpadding="2"  class="bodystyle">
<?php
			$row = $rows[0];
			foreach ($row as $key=>$value) {
			
				$value = preg_replace('/</', "&lt;", $value);
				$value = preg_replace('/>/', "&#62;", $value);
			
				if($key == "msg") {
					$value = preg_replace('/'.$row->method.'/', "<font color='red'><b>$row->method</b></font>", $value);				
					$value = preg_replace('/'.$row->via_1_branch.'/', "<font color='green'><b>$row->via_1_branch</b></font>", $value);				
					$value = preg_replace('/'.$row->callid.'/', "<font color='blue'><b>$row->callid</b></font>", $value);				
					$value = preg_replace('/'.$row->from_tag.'/', "<font color='red'><b>$row->from_tag</b></font>", $value);				
				#	$value = preg_replace('/'.$row->to_tag.'/', "<font color='darkblue'><b>$row->to_tag</b></font>", $value);				
				}			
						
				if($value == "") continue;

				$value = preg_replace('/\n/', "\n<BR>", $value);
				

				
?>							
			<tr >
			    <td align="right" class="dataTableContentBB"><b><?php echo $key;?></b></td>
			    <td align="left" class="dataTableContentBB"><font color="#000"><b><?php echo $value;?></b></font></td>
			 </tr>
<?php
			}
?>			 
		</table>		

<?php
	}


	function displayCallFlow($callid, $pcap_path) {
?>	
		<table border="0" cellspacing="2" cellpadding="2">
			<tr style="query_simple">
			    <td style="tablerow_one" align="right" class="dataTableContentB">Capturing:</td>
			    <td style="tablerow_one" align="left" class="dataTableContentBB"><a href="tmp/pcap/<?php echo $callid;?>/trace.pcap"><?php echo $callid?></a></td>
			 </tr>
		</table>		

<?php
	}


	function displayAdminOverView($type, $rows, $name, $dval, $count) {
	    $columns = $rows[0];
	    	    	    
?>	
<!- admin mod start -->
        <ul id="column<?php echo $count;?>" class="column" style="width: 80%;">
            <li class="widget color-orange" id="widget2">
                <div class="widget-head">
                    <h3>ADMIN - <?php echo $name?></h3>
                </div>
                <div class="widget-content">

	<br>
            <table border="1" id="data" cellspacing="0" width="95%" style="background: #f9f9f9;">            			
		<tr>
<?php
	     
        	   foreach ($columns as $key=>$value) {
          	    if($key == "id" || $key == "userid") continue;
			$ktitle = strtoupper($key);
                    echo "<th>$ktitle</th>";                
                }        
		echo "</tr>";

                foreach($rows as $row) {      
                    
                    echo "<tr align='center'>\n";
                      			
                    foreach($row as $key=>$value) {
                        if($key == "id" || $key == "userid") continue;          
                        $id = !isset($row->userid) ? $row->id : $row->userid;
                        //$id = $row->id;                        
                        
                        echo "<td class=\"editableSingle category{$key} removable id{$type}{$id}\">$value</td>\n";                    
                    }
                    echo "</tr>\n";
}
?>			    
            </table>               
	<br>
    	<div align="right"><button id="create-<?php echo $dval?>">Create New</button></div>          
 	<br>
	</center>
</li></ul>
<!-- admin mod end -->               

<?php
        }


        function displayStats() {
?>

<!- stats mod start -->
  <div id="columns">
        <center>
        <ul id="column1" class="column" style="width: 10%;">
        </ul>

        <ul id="column2" class="column" style="width: 80%;">
            <li class="widget color-yellow" id="widget2">
                <div class="widget-head">
                    <h3>Capture Stats</h3>
                </div>
                <div class="widget-content">


        <div id="Modules"></div><br>

	<script type="text/javascript">
        jQuery(document).ready( function($) {

<?php

        $submodules = array_filter(glob('modules/*'), 'is_dir');
        $modcount = 0;
        foreach( $submodules as $key => $value){
?>

                $('#Modules').append('<iframe id="stats<?php echo $modcount ?>" frameborder="0" scrolling="no" style="width:95%;height: 250px;overflow: auto;" />');
                $('#stats<?php echo $modcount ?>').attr('src', '<?php echo $value ?>');


<?php
        $modcount++;
        }

?>
                });
        </script>
	
</div>



<?php
        }

  function displayNewAdminOverView($allrows, $allnames, $task, $alldval) {

        global $mynodeshost, $db, $task;
        
	include("admin.html.php");

  }

  function displayToolBox() {

        include("toolbox.html.php");

  }


}

?>


