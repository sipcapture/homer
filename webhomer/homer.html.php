<?php

defined( '_HOMEREXEC' ) or die( 'Restricted access' );

class HTML_adminhomer {

        function displayStart ($status, $header,$task, $level) {

        ?>
        <html xmlns="http://www.w3.org/1999/xhtml">
            <head>
            <meta http-equiv="Content-Type" content="text/html; charset=utf-8" />
            <title>Web Homer - <?php echo $status;?> </title>
            <link href="styles/core_styles.css" rel="stylesheet" type="text/css" />
            <link href="styles/form.css" rel="stylesheet" type="text/css" />
            <link href="styles/jquery.timeentry.css" rel="stylesheet" type="text/css" />            
            <link type="text/css" href="styles/jquery-ui-1.8.4.custom.css" rel="stylesheet" />	
            
            </head>
            
            <body>          

            <script type="text/javascript" src="js/homer.js"></script>
            <!-- <script src="js/jquery-1.5.1.min.js" type="text/javascript"></script> -->
            <script src="js/jquery-1.6.4.min.js" type="text/javascript"></script>
            <script type="text/javascript" src="js/jquery-ui-1.8.16.custom.min.js"></script>
<?php

if($header) {
?>         
	<div id="newbg" class="newbg"><img src="images/bg.gif" width="100%" height="100%"></div>
	<div id="banner"> 
		<h1 class="logo"> 
			<a href="homer.php" title="Homer Capture Server"><span>Homer</span></a> 
		</h1> 
		<div id="dock"> 
			<div class="left"></div> 
			<ul> 
				<li <?php if($task=="search") echo "class='selected'"; ?>> 
				    <a href="homer.php?task=search">Search</a> 
				</li> 
				<li <?php if($task=="advsearch") echo "class='selected'"; ?>><a href="homer.php?task=advsearch">Advanced Search</a> 
				</li> 
				
				<li <?php if($task=="stats") echo "class='selected'"; ?>> <a href="homer.php?task=stats">Stats</a>
                                </li>

<?php if($level == 1):?>				
				<li <?php if(!strncmp(admin,$task,5)) echo "class='selected'"; ?>> <a href="homer.php?task=adminoverview">Admin</a> 
				</li> 
<?php endif;?>				
				<li style="padding-left: 12px;"> 
					<div style="background: #555; width: 1px; height: 24px; position: absolute; left: 0px;"></div> 
					<a href="homer.php?task=logout">Logout</a> 
				</li> 
			</ul> 
			<div class="right"></div> 
		</div> 
		<div id="navigation"> 
			<div class="left"></div> 
			    <ul id="icons" class="ui-widget ui-helper-clearfix">
<?php if($task == "result"):?>			    
                            <li class="ui-state-default ui-corner-all" id="setupheader" title="Show Columns"><span class="ui-icon ui-icon-wrench"></span></li>
                            <li class="ui-state-default ui-corner-all" onClick="javascript:showSearch();" title="Field Search"><span class="ui-icon ui-icon-search"></span></li>
<?php endif; ?>			                    
<?php if(!strncmp("admin", $task, 5)):?>			                
                            <a href="homer.php?task=adminusers"><li class="ui-state-default ui-corner-all" title="Users"><span class="ui-icon ui-icon-person"></span></li></a>
                            <a href="homer.php?task=adminnodes"><li class="ui-state-default ui-corner-all"title="Hosts"><span class="ui-icon ui-icon-calculator"></span></li></a>
                            <a href="homer.php?task=adminhosts"><li class="ui-state-default ui-corner-all"title="Nodes"><span class="ui-icon ui-icon-gear"></span></li></a>
<?php endif; ?>			                    
                            </ul>
			<div class="right"></div> 
		</div> 
	</div> 
	<div id="content-wrapper"> 
		<div id="content"> 
		<div class="content-top"></div>		<div class="content"> 
<script type="text/javascript"> 
var section = "demos/dialog";
</script> 



<?php
}
?>
            <form action="homer.php" method="POST" name="homer" id="homer">
            
        <?php

        }

	function displayStop () {
        ?>
		<input type="hidden" name="task" id="task" value="result">
        	</form>                
            </body></html>
        <?php
        }

	function displaySearchForm ($search, $nodes) {

?>	
		<script src="js/jquery.timeentry.js" type="text/javascript"></script>
		<script src="js/jquery.mousewheel.js" type="text/javascript"></script>
		<script src="js/jquery.timeentry-de.js" type="text/javascript"></script>
				                           
	         <script type="text/javascript">
	         
	         	$.noConflict();
	         	
	         	jQuery(document).ready( function($) {
		                $('.timepicker1').timeEntry({show24Hours: true, showSeconds: true}); 
		                $('.timepicker2').timeEntry({show24Hours: true, showSeconds: true}); 
		                
				$('#from_user').autocomplete({
                                source: "utils.php?task=livesearch&field=from_user&date="+$('#date').val()+"&from_time="+$('#from_time').val()+"&to_time="+$('#to_time').val(),
                                minLength: 3,
	                        });
	                        
        	                $('#to_user').autocomplete({
                                source: "utils.php?task=livesearch&field=to_user&date="+$('#date').val()+"&from_time="+$('#from_time').val()+"&to_time="+$('#to_time').val(),
                                minLength: 3,
                                select: function(event, ui) {
                                        $('#to_user').val(ui.item.to_user);
                                }
                                });                                
			});

	        
		</script>	                	    
    	        

<br>
	<div class="wrapper">
	<div id="results">
	<!-- extra formatting table -->
	<table border=0 width="100%"><tr><td>
	     <fieldset class="adminform">
             <legend>Call Details</legend>                                                                                                      		
			<table class="bodystyle" height="130">
				<tr>
					<td width="150" class="tablerow_two">
						<label for="ruri_user" title="B-Number in Request URI user part">RURI User (B-Number)</label>
					</td>
					<td>
						<input type="text" name="ruri_user" id="ruri_user" class="textfieldstyle" size="40" value="<?php if(isset($search['ruri_user'])) echo $search['ruri_user']; ?>" />
					</td>
				</tr>
				<tr>
					<td width="150" class="tablerow_two">
						<label for="to_user">To User (B-Number)</label>
					</td>
					<td>
						<input type="text" name="to_user" id="to_user" class="textfieldstyle" size="40" value="<?php if(isset($search['to_user'])) echo $search['to_user']; ?>" />
					</td>
				</tr>
				<tr>
					<td width="150" class="tablerow_two">
						<label for="from_user">From User (A-Number)</label>
					</td>
					<td>
						<input type="text" name="from_user" id="from_user" class="textfieldstyle" size="40" value="<?php if(isset($search['from_user'])) echo $search['from_user']; ?>" />
					</td>
				</tr>
				<tr>
					<td width="150" class="tablerow_two">
						<label for="pid_user" title="P-Asserted and P-Preffered">PID User (A-Number)</label>
					</td>
					<td>
						<input type="text" name="pid_user" id="pid_user" class="textfieldstyle" size="40" value="<?php if(isset($search['pid_user'])) echo $search['pid_user']; ?>" />
					</td>
				</tr>
				<tr>
					<td width="150" class="tablerow_two">
						<label for="logic_or" title="Logic OR">Logic OR in search</label>
					</td>
					<td>
						<input type="checkbox" name="logic_or" id="logic_or" class="checkboxstyle" value="1" <?php if(isset($search['logic_or']) && $search['logic_or'] == 1) echo "checked"; ?> />
					</td>
				</tr>				

				<tr>
					<td width="150" class="tablerow_two">
						<label for="callid" title="Callid">Call-ID</label>
					</td>
					<td>
						<input type="text" name="callid" id="callid" class="textfieldstyle" size="40" value="<?php if(isset($search['callid'])) echo $search['callid']; ?>" />
						<input type="hidden" name="method" id="method" class="textfieldstyle" size="40" value="" />
					</td>
				</tr>
			</table>	
		</fieldset>

		</td><td>

		 <fieldset class="adminform">
                 <legend>Time/Date Parameters</legend>                 
                 		<table class="bodystyle"  height="130">						
							<tr>
							
								<td width="40%" class="paramlist_key"><label for="location" title=".">Location</label></td>
								<td class="tablerow_two">
								<?php
								
								    if(isset($search['location'])) $locarray = $search['location'];
								    else $locarray = array();
								    
								    foreach ($nodes as $key=>$value) {
                                                                ?>								
								        <input type="checkbox" class="checkboxstyle" name="location[]" id="location" value="1" <?php if(in_array($key,$locarray)) echo "checked";?>><?php echo $value;?>
                                                                <?php
                                                                    }
                                                                ?>
								</td>
							</tr>
							<tr>
								<td width="40%" class="paramlist_key"><label for="date" title=".">Date</label></td>
								<td class="tablerow_two">
									<select name="date" id="date" class="dropdownstyle">
								<?php
									$sCurrentDate = date("d-m-Y");
									$wday = date("N");
									
									for($i=0; $i < 7; $i++) {
									
										if($i==0) $namedate = "Today [".$sCurrentDate."]";
										else if($i==1) $namedate = "Yesterday [".$sCurrentDate."]";
										else $namedate = "$dow [".$sCurrentDate."]";		
																													
								?>									
										<option value="<?php echo $sCurrentDate;?>" <?php if(isset($search['date']) && $sCurrentDate == $search['date']) echo "selected"  ?>><?php echo $namedate;?></option>
								<?php										
										$sCurrentDate = date("d-m-Y", strtotime("-1 day", strtotime($sCurrentDate)));
										$dow = date("l", strtotime($sCurrentDate));
									}
								?>
									</select>
								</td>
							</tr>
							<?php
									$ft = date("H:i:s", strtotime("-1 hour"));
									$tt = date("H:i:s");
							
							
							?>
							<tr>
								<td width="40%" class="paramlist_key"><label for="time" title=".">From Time</label></td>
								<td class="tablerow_two">
									<input type="text" name="from_time" id="from_time" class="textfieldstyle2 timepicker1" size="6" value="<?php if(isset($search['from_time'])) echo $search['from_time']; else echo $ft;?>" />
								</td>
							</tr>
							<tr>
								<td width="40%" class="paramlist_key"><label for="time" title=".">To Time</label></td>
								<td class="tablerow_two">
									<input type="text" name="to_time" id="to_time" class="textfieldstyle2 timepicker2" size="6" value="<?php if(isset($search['to_time'])) echo $search['to_time']; else echo $tt; ?>" />
								</td>
							</tr>
                                                        <!--
							<tr>
								<td width="40%" class="paramlist_key"><label for="maximum" title=".">Maximum records</label></td>
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
						</table>					
			</fieldset>

			</td></tr><tr><td>
			<fieldset class="adminform">			
		        <legend>Search</legend>
			<table class="bodystyle" height="30">
				<tr>
					<td>
						<input type="submit" value="Search" onClick="return check_form();">
					</td>
					<td>
						<input type="button" value="Clear" onClick="clear_form();">
					</td>
					 </td>


                                        </td>

				</tr>
			</table>
			</fieldset>
			</td><td>
			 <fieldset class="adminform">
                        <legend>Network Status</legend>
                        <table class="bodystyle" height="30">
                                <tr>
                                        <td>

                                        </td>
                                        <td>
                	<?php
			echo "Up ";
			passthru("/usr/bin/uptime |  awk '{print $3, $4}'");
			passthru('/sbin/ifconfig eth0|grep "RX bytes"');
			?>
                                        </td>
                                </tr>
                        </table>
                        </fieldset>
			</td></td>
		</table>
			


	</div>
	<div id="Modules"></div>

<?php 

	if (MODULES != 0) {

	// Scan Modules directory and display
	$submodules = array_filter(glob('modules/*'), 'is_dir');
	$modcount = 0;
	  foreach( $submodules as $key => $value){
?>
	  <script type="text/javascript">
	  jQuery(document).ready( function($) {

		$('#Modules').append('<iframe id="stats<?php echo $modcount ?>" frameborder="0" scrolling="no" style="width:100%; height:315px;" />'); 
		$('#stats<?php echo $modcount ?>').attr('src', '<?php echo $value ?>'); 

		});
          </script>
<?php
	  $modcount++;
	  }

	}

?>


	<span class="note">
	<br />
	</div>
																														
<?php
	}


	function displayAdvanceSearchForm ($search, $nodes) {
	
	
?>	

		<script src="js/jquery.timeentry.js" type="text/javascript"></script>
		<script src="js/jquery.mousewheel.js" type="text/javascript"></script>
		<script src="js/jquery.timeentry-de.js" type="text/javascript"></script>
		                           
	         <script type="text/javascript">
	         
	         	$.noConflict();
	         	
	         	jQuery(document).ready( function($) {
		                $('.timepicker1').timeEntry({show24Hours: true, showSeconds: true}); 
		                $('.timepicker2').timeEntry({show24Hours: true, showSeconds: true}); 
			});
	        
		</script>	                	        	        

<div class="wrapper">

		 <!-- extra formatting table -->
        	<table border=0 width="100%"><tr><td>
		
		<fieldset class="adminform">
		<legend>User Details</legend>
			<table class="bodystyle" height="130">			             
				<tr>
					<td width="150" class="tablerow_two">
						<label for="ruri_user" title="B-Number in Request URI user part">RURI User (B-Number)</label>
					</td>
					<td>
						<input type="text" name="ruri_user" id="ruri_user" class="textfieldstyle2" size="60" value="<?php if(isset($search['ruri_user'])) echo $search['ruri_user']; ?>" />
					</td>
				</tr>
				<tr>
					<td width="150" class="tablerow_two">
						<label for="to_user">To User (B-Number)</label>
					</td>
					<td>
						<input type="text" name="to_user" id="to_user" class="textfieldstyle2" size="60" value="<?php if(isset($search['to_user'])) echo $search['to_user']; ?>" />
					</td>
				</tr>
				<tr>
					<td width="150" class="tablerow_two">
						<label for="from_user">From User (A-Number)</label>
					</td>
					<td>
						<input type="text" name="from_user" id="from_user" class="textfieldstyle2" size="60" value="<?php if(isset($search['from_user'])) echo $search['from_user']; ?>" />
					</td>
				</tr>
				<tr>
					<td width="150" class="tablerow_two">
						<label for="pid_user" title="P-Asserted and P-Preffered">PID User (A-Number)</label>
					</td>
					<td>
						<input type="text" name="pid_user" id="pid_user" class="textfieldstyle2" size="60" value="<?php if(isset($search['pid_user'])) echo $search['pid_user']; ?>" />
					</td>
				</tr>
				<tr>
					<td width="150" class="tablerow_two">
						<label for="contact_user" title="Contact header"">Contact User</label>
					</td>
					<td>
						<input type="text" name="contact_user" id="contact_user" class="textfieldstyle2" size="60" value="<?php if(isset($search['contact_user'])) echo $search['contact_user']; ?>" />
					</td>
				</tr>
				<tr>
					<td width="150" class="tablerow_two">
						<label for="auth_user" title="Proxy-Auth, WWW-Auth">Auth User</label>
					</td>
					<td>
						<input type="text" name="auth_user" id="auth_user" class="textfieldstyle2" size="60" value="<?php if(isset($search['auth_user'])) echo $search['auth_user']; ?>" />
					</td>
				</tr>
				<tr>
					<td width="150" class="tablerow_two">
						<label for="logic_or" title="Logic OR">Logic OR in search</label>
					</td>
					<td>
						<input type="checkbox" name="logic_or" id="logic_or" class="checkboxdstyle" value="1" <?php if(isset($search['logic_or']) && $search['logic_or'] == 1) echo "checked"; ?> />
					</td>
				</tr>				
			</table>
		</fieldset>

		</td><td>

		<fieldset class="adminform">
		<legend>Call Details</legend>
			<table class="bodystyle" height="130">
				<tr>
					<td width="150" class="tablerow_two">
						<label for="callid" title="Callid">Call-ID</label>
					</td>
					<td>
						<input type="text" name="callid" id="callid" class="textfieldstyle" size="40" value="<?php if(isset($search['callid'])) echo $search['callid']; ?>" />
					</td>
				</tr>
				<tr>
					<td width="150" class="tablerow_two">
						<label for="callid_aleg" title="Search Call-ID as bridged call (Ariadne)">B2B Call-ID</label>
					</td>
					<td>
						<input type="checkbox" name="callid_aleg" id="callid_aleg" class="checkboxdstyle" value="1" <?php if(isset($search['callid_aleg']) && $search['callid_aleg'] == 1) echo "checked"; ?>/>
					</td>
				</tr>
				<tr>
					<td width="150" class="tablerow_two">
						<label for="from_user">From Tag</label>
					</td>
					<td>
						<input type="text" name="from_tag" id="from_tag" class="textfieldstyle" size="40" value="<?php if(isset($search['from_tag'])) echo $search['from_tag']; ?>" />
					</td>
				</tr>
				<tr>
					<td width="150" class="tablerow_two">
						<label for="to_tag" title="To TAG">To Tag</label>
					</td>
					<td>
						<input type="text" name="to_tag" id="to_tag" class="textfieldstyle" size="40" value="<?php if(isset($search['to_tag'])) echo $search['to_tag']; ?>" />
					</td>
				</tr>
				<tr>
					<td width="150" class="tablerow_two">
						<label for="via_1_branch" title="Via branch"">Via Branch</label>
					</td>
					<td>
						<input type="text" name="via_1_branch" id="via_1_branch" class="textfieldstyle" size="40" value="<?php if(isset($search['via_1_branch'])) echo $search['via_1_branch']; ?>" />
					</td>
				</tr>
				<tr>
					<td width="150" class="tablerow_two">
						<label for="method" title="INVITE, REGISTER, NOTIFY, 401, 200">Method / Reply</label>
					</td>
					<td>
						<input type="text" name="method" id="method" class="textfieldstyle" size="40" value="<?php if(isset($search['method'])) echo $search['method']; ?>" />
					</td>
				</tr>
				<tr>
					<td width="150" class="tablerow_two">
						<label for="reply_reason" title="OK, Not allowed">Reply reason</label>
					</td>
					<td>
						<input type="text" name="reply_reason" id="reply_reason" class="textfieldstyle" size="40" value="<?php if(isset($search['reply_reason'])) echo $search['reply_reason']; ?>" />
					</td>
				</tr>				
			</table>
		</fieldset>

		</td></tr><tr><td>

		<fieldset class="adminform">
		<legend>Header Details</legend>
			<table class="bodystyle"  height="130">
				<tr>
					<td width="150" class="tablerow_two">
						<label for="ruri" title="RURI">RURI</label>
					</td>
					<td>
						<input type="text" name="ruri" id="ruri" class="textfieldstyle2" size="60" value="<?php if(isset($search['ruri'])) echo $search['ruri']; ?>" />
					</td>
				</tr>
				<tr>
					<td width="150" class="tablerow_two">
						<label for="via_1" title="VIA">VIA 1</label>
					</td>
					<td>
						<input type="text" name="via_1" id="via_1" class="textfieldstyle2" size="60" value="<?php if(isset($search['via_1'])) echo $search['via_1']; ?>" />
					</td>
				</tr>	
				<tr>
					<td width="150" class="tablerow_two">
						<label for="diversion" title="Diversion">Diversion</label>
					</td>
					<td>
						<input type="text" name="diversion" id="diversion" class="textfieldstyle2" size="60" value="<?php if(isset($search['diversion'])) echo $search['diversion']; ?>" />
					</td>
				</tr>	
				<tr>
					<td width="150" class="tablerow_two">
						<label for="cseq" title="Cseq">Cseq</label>
					</td>
					<td>
						<input type="text" name="cseq" id="cseq" class="textfieldstyle" size="40" value="<?php if(isset($search['cseq'])) echo $search['cseq']; ?>" />
					</td>
				</tr>	
				<tr>
					<td width="150" class="tablerow_two">
						<label for="reason" title="Reason">Reason</label>
					</td>
					<td>
						<input type="text" name="reason" id="reason" class="textfieldstyle2" size="60" value="<?php if(isset($search['reason'])) echo $search['reason']; ?>" />
					</td>
				</tr>	
				<tr>
					<td width="150" class="tablerow_two">
						<label for="content-type" title="Content-Type">Content-Type</label>
					</td>
					<td>
						<input type="text" name="content_type" id="content_type" class="textfieldstyle2" size="60" value="<?php if(isset($search['content_type'])) echo $search['content_type']; ?>" />
					</td>
				</tr>
				<tr>
					<td width="150" class="tablerow_two">
						<label for="authorization" title="Authorization">Authorization</label>
					</td>
					<td>
						<input type="text" name="authorization" id="authorization" class="textfieldstyle2" size="60" value="<?php if(isset($search['authorization'])) echo $search['authorization']; ?>" />
					</td>
				</tr>	
				<tr>
					<td width="150" class="tablerow_two">
						<label for="user_agent" title="User-Agent">User-Agent</label>
					</td>
					<td>
						<input type="text" name="user_agent" id="user_agent" class="textfieldstyle2" size="60" value="<?php if(isset($search['user_agent'])) echo $search['user_agent']; ?>" />
					</td>
				</tr>															
			</table>
		</fieldset>	

		</td><td valign="top">	
		                		
		<fieldset class="adminform">
		<legend>Time / Date Parameters</legend>
			<table class="bodystyle"  height="130">
				<tr>
					<td>
						<table width="100%" class="paramlist bodystyle" cellspacing="1">
							<tr>
								<td width="40%" class="paramlist_key"><label for="location" title=".">Location</label></td>
								<td class="tablerow_two">
								<?php
								    foreach ($nodes as $key=>$value) {
                                                                ?>								
								        <input type="checkbox" class="checkboxstyle" name="location[]" id="location" value="1" <?if(in_array($key,$search['location'])) echo "checked";?>><?php echo $value;?>
                                                                <?php
                                                                    }
                                                                ?>
								</td>
							</tr>
							<tr>
								<td width="40%" class="paramlist_key"><label for="date" title=".">Date</label></td>
								<td class="tablerow_two">
									<select name="date" id="date" class="dropdownstyle">
								<?php
									$sCurrentDate = date("d-m-Y");
									$wday = date("N");
									
									for($i=0; $i < 7; $i++) {
									
										if($i==0) $namedate = "Today [".$sCurrentDate."]";
										else if($i==1) $namedate = "Yesterday [".$sCurrentDate."]";
										else $namedate = "$dow [".$sCurrentDate."]";		
																													
								?>									
										<option value="<?php echo $sCurrentDate;?>" <?php if(isset($search['date']) && $sCurrentDate == $search['date']) echo "selected"  ?>><?php echo $namedate;?></option>
								<?php										
										$sCurrentDate = date("d-m-Y", strtotime("-1 day", strtotime($sCurrentDate)));
										$dow = date("l", strtotime($sCurrentDate));
									}
								?>
									</select>
								</td>
							</tr>
							<?php
									$ft = date("H:i:s", strtotime("-1 hour"));
									$tt = date("H:i:s");
							
							
							?>
							<tr>
								<td width="40%" class="paramlist_key"><label for="time" title=".">From Time</label></td>
								<td class="tablerow_two">
									<input type="text" name="from_time" id="from_time" class="textfieldstyle2 timepicker1" size="10" value="<?php if(isset($search['from_time'])) echo $search['from_time']; else echo $ft;?>" />
								</td>
							</tr>
							<tr>
								<td width="40%" class="paramlist_key"><label for="time" title=".">To Time</label></td>
								<td class="tablerow_two">
									<input type="text" name="to_time" id="to_time" class="textfieldstyle2 timepicker2" size="10" value="<?php if(isset($search['to_time'])) echo $search['to_time']; else echo $tt; ?>" />
								</td>
							</tr>
                                                        <!--
							<tr>
								<td width="40%" class="paramlist_key"><label for="maximum" title=".">Maximum records</label></td>
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
						</table>					
					</td>
				</tr>
			</table>
		</fieldset>

		</td></tr><tr><td>

		<fieldset class="adminform">
		<legend>Network Details</legend>
			<table class="bodystyle" cellspacing="1">
				<tr>
					<td width="150" class="tablerow_two">
						<label for="source_ip" title="Source IP">Source IP</label>
					</td>
					<td>
						<input type="text" name="source_ip" id="source_ip" class="textfieldstyle" size="40" value="<?php if(isset($search['source_ip'])) echo $search['source_ip']; ?>" />
					</td>
					<td width="150" class="tablerow_two">
						<label for="source_port" title="Source PORT">Source port</label>
					</td>
					<td>
						<input type="text" name="source_port" id="source_port" class="textfieldstyle2" size="5" value="<?php if(isset($search['source_port'])) echo $search['source_port']; ?>" />
					</td>
				</tr>
				<tr>
					<td width="150" class="tablerow_two">
						<label for="destination_ip" title="Destination IP">Destination IP</label>
					</td>
					<td>
						<input type="text" name="destination_ip" id="destination_ip" class="textfieldstyle" size="40" value="<?php if(isset($search['destination_ip'])) echo $search['destination_ip']; ?>" />
					</td>
					<td width="150" class="tablerow_two">
						<label for="destination port" title="Destination PORT">Dest. port</label>
					</td>
					<td>
						<input type="text" name="destination_port" id="destination_port" class="textfieldstyle2" size="5" value="<?php if(isset($search['destination_port'])) echo $search['destination_port']; ?>" />
					</td>
				</tr>	
				<tr>
					<td width="150" class="tablerow_two">
						<label for="contact_ip" title="Contact IP">Contact IP</label>
					</td>
					<td>
						<input type="text" name="contact_ip" id="contact_ip" class="textfieldstyle" size="40" value="<?php if(isset($search['contact_ip'])) echo $search['contact_ip']; ?>" />
					</td>
					<td width="150" class="tablerow_two">
						<label for="contact port" title="Contact PORT">Contact port</label>
					</td>
					<td>
						<input type="text" name="contact_port" id="contact_port" class="textfieldstyle2" size="5" value="<?php if(isset($search['contact_port'])) echo $search['contact_port']; ?>" />
					</td>
				</tr>
				<tr>
					<td width="150" class="tablerow_two">
						<label for="originator_ip" title="Originator IP">Originator IP</label>
					</td>
					<td>
						<input type="text" name="originator_ip" id="originator_ip" class="textfieldstyle" size="40" value="<?php if(isset($search['originator_ip'])) echo $search['originator_ip']; ?>" />
					</td>
					<td width="150" class="tablerow_two">
						<label for="originator port" title="Originator PORT">Originator port</label>
					</td>
					<td>
						<input type="text" name="originator_port" id="originator_port" class="textfieldstyle2" size="5" value="<?php if(isset($search['originator_port'])) echo $search['originator_port']; ?>" />
					</td>
				</tr>				
				<tr>
					<td width="40%" class="paramlist_key"><label for="proto" title=".">Proto</label></td>
						<td class="tablerow_two">
							<select name="proto" id="proto" class="dropdownstyle">
							        <option value="1" >TCP</option>
								<option value="2" selected="selected">UDP</option>
								<option value="3" >TLS/SSL</option>
								<option value="4" >SCTP</option>
							</select>
					</td>
				</tr>
				<tr>
					<td width="40%" class="paramlist_key"><label for="family" title=".">Family</label></td>
						<td class="tablerow_two">
							<select name="family" id="family" class="dropdownstyle">
								<option value="1" selected="selected">IPv4</option>
								<option value="2" >IPv6</option>
							</select>
					</td>
				</tr>
			</table>
		</fieldset> 

		</td><td valign="bottom">

		<fieldset class="adminform">
			<table class="bodystyle">
				<tr>
					<td>
						<br />
						<input type="submit" value="Search" onClick="return check_form();">
					</td>
					<td>
						<br />
						<input type="button" value="Clear" onClick="clear_complete_form();">
					</td>
				</tr>
			</table>
				</fieldset>

		</td></tr></table>
																						<br />
																												<span class="note">
		
	</div>
</div>
<?php
	}

	function displayResultSearch ($table,$ft,$tt) {
	
		//Color Generator
		srand(floor(time() / (60*60*24)));
		$hex = array("00", "33", "66", "99", "CC", "FF");	 					
?>		
            <style type="text/css" title="currentStyle">
			@import "styles/demo_page.css";
                        @import "styles/demo_table.css";
                        @import "styles/demo_table_jui.css";
            </style>            

	<script type="text/javascript" language="javascript" src="js/jquery.dataTables.js"></script>        
        <script type="text/javascript" src="js/jquery.Dialog.js"></script>

	<script type="text/javascript">		
	 	$(document).mousemove(function(e){
                                        $('body').data('posx', e.pageX);
                                        $('body').data('posy', e.pageY);
                        });
	</script>


 <table border="0" cellpadding="0" cellspacing="0" bgcolor="#eeeeee">
        <tr>
                <td style="border: 1px solid #C0C0C0"><font size="4"><b>&#916;</b></font>
                        <input type="text" name="Display" id="delta_value_1" align="right" class="textfieldstyle2"></td>
                <td style="border: 1px solid #C0C0C0; width: 20px; font-size: 20px; font-weight: bold; text-align: center;"> - </td>
                <td style="border: 1px solid #C0C0C0">
                        <input type="text" name="Display2" id="delta_value_2" align="right" class="textfieldstyle2"></td>
                <td style="border: 1px solid #C0C0C0; width: 20px; font-size: 20px; font-weight: bold; text-align: center;"> = </td>
                <td style="border: 1px solid #C0C0C0"><input type="text" id="delta_result" name="Display3" align="right" size="10" class="textfieldstyle2"> &micro;</td>
                <td>&nbsp;&nbsp;</td>
                <td style="border: 1px solid #C0C0C0;text-align: right">Result: <b><?php echo date("d-m-Y H:i:s", strtotime($ft));?> - <?php echo date("d-m-Y H:i:s", strtotime($tt));?></b> </td>
        </tr>
</table>


        <?php echo $table->render(); ?>

        <script type="text/javascript" language="javascript">
                
        function fnShowHide( iCol ) {
		/* Get the DataTables object again - this is not a recreation, just a get of the object */
		var oTable = $('#demoTable').dataTable();	
		var bVis = oTable.fnSettings().aoColumns[iCol].bVisible;
		oTable.fnSetColumnVis( iCol, bVis ? false : true );
	}
        
        </script>

<div id="dialog-form" title="Select fields to show">
	<p class="validateTips">Click on checkbox to hide/show.</p>

	<form>
	<fieldset>

	<table border="0" class="layout-grid" width="100%">
	<tr>
	    <td><input type="checkbox" onClick="fnShowHide(1); " class="text ui-widget-content ui-corner-all" checked/></td>
	    <td>ID</td>	    
	    <td><input type="checkbox" onClick="fnShowHide(2); " class="text ui-widget-content ui-corner-all" checked/></td>	    	    
	    <td>Date</td>
	    <td><input type="checkbox" onClick="fnShowHide(3); " class="text ui-widget-content ui-corner-all"/></td>	    
	    <td>Timestamp</td>	    
	    <td><input type="checkbox" onClick="fnShowHide(4); " class="text ui-widget-content ui-corner-all" checked/></td>	    
	    <td>Method</td>
        </tr>
        <tr>
	    <td><input type="checkbox" onClick="fnShowHide(5); " class="text ui-widget-content ui-corner-all" /></td>
	    <td>Reply Reason</td>	    
	    <td><input type="checkbox" onClick="fnShowHide(6); " class="text ui-widget-content ui-corner-all"/></td>	    	    
	    <td>Request URI</td>
	    <td><input type="checkbox" onClick="fnShowHide(7); " class="text ui-widget-content ui-corner-all" checked/></td>	    
	    <td>RURI User</td>	    
	    <td><input type="checkbox" onClick="fnShowHide(8); " class="text ui-widget-content ui-corner-all" checked/></td>	    
	    <td>From User</td>
        </tr>
	<tr>
	    <td><input type="checkbox" onClick="fnShowHide(9);" class="text ui-widget-content ui-corner-all"/></td>
	    <td>From Tag</td>	    
	    <td><input type="checkbox" onClick="fnShowHide(10);" class="text ui-widget-content ui-corner-all" checked/></td>	    	    
	    <td>To User</td>
	    <td><input type="checkbox" onClick="fnShowHide(11); " class="text ui-widget-content ui-corner-all"/></td>	    
	    <td>To Tag</td>	    
	    <td><input type="checkbox" onClick="fnShowHide(12); " class="text ui-widget-content ui-corner-all" checked/></td>	    
	    <td>Pid User</td>
        </tr>
        <tr>
	    <td><input type="checkbox" onClick="fnShowHide(13); " class="text ui-widget-content ui-corner-all"/></td>	    	    
	    <td>Contact User</td>
	    <td><input type="checkbox" onClick="fnShowHide(14); " class="text ui-widget-content ui-corner-all"/></td>	    
	    <td>Auth User</td>	    
	    <td><input type="checkbox" onClick="fnShowHide(15); " class="text ui-widget-content ui-corner-all" checked/></td>	    
	    <td>Call-ID</td>
	    <td><input type="checkbox" onClick="fnShowHide(16); " class="text ui-widget-content ui-corner-all"/></td>
	    <td>Call-ID Aleg</td>	    	    
        </tr>
	<tr>
	    <td><input type="checkbox" onClick="fnShowHide(17); " class="text ui-widget-content ui-corner-all"/></td>	    	    
	    <td>Via 1</td>
	    <td><input type="checkbox" onClick="fnShowHide(18); " class="text ui-widget-content ui-corner-all"/></td>	    
	    <td>Via 1 branch</td>	    
	    <td><input type="checkbox" onClick="fnShowHide(19); " class="text ui-widget-content ui-corner-all"/></td>	    
	    <td>Cseq</td>
	    <td><input type="checkbox" onClick="fnShowHide(20); " class="text ui-widget-content ui-corner-all"/></td>
	    <td>Diversion</td>	    	    
        </tr>
        <tr>
	    <td><input type="checkbox" onClick="fnShowHide(21); " class="text ui-widget-content ui-corner-all"/></td>	    	    
	    <td>Reason</td>
	    <td><input type="checkbox" onClick="fnShowHide(22); " class="text ui-widget-content ui-corner-all"/></td>	    
	    <td>Content-Type</td>	    
	    <td><input type="checkbox" onClick="fnShowHide(23); " class="text ui-widget-content ui-corner-all"/></td>	    
	    <td>Authorization</td>
	    <td><input type="checkbox" onClick="fnShowHide(24); " class="text ui-widget-content ui-corner-all"/></td>
	    <td>User Agent</td>	    	    
        </tr>        
        <tr>
	    <td><input type="checkbox" onClick="fnShowHide(25); " class="text ui-widget-content ui-corner-all" checked/></td>	    	    
	    <td>Source IP</td>
	    <td><input type="checkbox" onClick="fnShowHide(26); " class="text ui-widget-content ui-corner-all" checked/></td>	    
	    <td>Source Port</td>	    
	    <td><input type="checkbox" onClick="fnShowHide(27); " class="text ui-widget-content ui-corner-all" checked/></td>	    
	    <td>Dest. IP</td>
	    <td><input type="checkbox" onClick="fnShowHide(28); " class="text ui-widget-content ui-corner-all" checked/></td>
	    <td>Dest. Port</td>	    	    
        </tr>                
        <tr>
	    <td><input type="checkbox" onClick="fnShowHide(29); " class="text ui-widget-content ui-corner-all"/></td>	    	    
	    <td>Contact IP</td>
	    <td><input type="checkbox" onClick="fnShowHide(30); " class="text ui-widget-content ui-corner-all"/></td>	    
	    <td>Contact Port</td>	    
	    <td><input type="checkbox" onClick="fnShowHide(31); " class="text ui-widget-content ui-corner-all"/></td>	    
	    <td>Origin. IP</td>
	    <td><input type="checkbox" onClick="fnShowHide(32); " class="text ui-widget-content ui-corner-all"/></td>
	    <td>Origin. Port</td>	    	    
        </tr>                
        <tr>
	    <td><input type="checkbox" onClick="fnShowHide(33); " class="text ui-widget-content ui-corner-all"/></td>	    	    
	    <td>Proto</td>
	    <td><input type="checkbox" onClick="fnShowHide(34); " class="text ui-widget-content ui-corner-all"/></td>	    
	    <td>Family</td>	    
	    <td><input type="checkbox" onClick="fnShowHide(35); " class="text ui-widget-content ui-corner-all"/></td>	    
	    <td>Type</td>
	    <td><input type="checkbox" onClick="fnShowHide(36); " class="text ui-widget-content ui-corner-all" checked/></td>
	    <td>Node</td>	    	    
        </tr>        
        
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


	function displayAdminOverView($type, $rows, $name, $task, $dval) {
	    $columns = $rows[0];
	    	    	    
?>	
    </form>
        <script type="text/javascript" src="js/jquery.Dialog.js"></script>
        <script type="text/javascript" src="js/jquery.inlineEdit.js"></script>
        <script type="text/javascript">
            $(function(){
                $.inlineEdit({
			categoryhost: 'adminajax.php?type=host&categoryId=',			
			categoryname: 'adminajax.php?type=name&categoryId=',
			categorystatus: 'adminajax.php?type=status&categoryId=',
			categorypassword: 'adminajax.php?type=password&categoryId=',			
			categoryuseremail: 'adminajax.php?type=useremail&categoryId=',
			categoryuserlevel: 'adminajax.php?type=userlevel&categoryId=',			
			remove: 'adminajax.php?type=remove&categoryId=',
	
		}, {
	
			animate: false,
	
			filterElementValue: function($o){
                                return $o.html();
			},
	
			afterSave: function(o){
				if (o.type == 'category2name') {
					$('.category2name.id' + o.id).prepend('$');
			}
		}	            
                });
            });
        
        </script>
        <style type="text/css"> 
		#data td {
			width: 150px;
			vertical-align: top;
			cursor: pointer;
		}
		.editFieldSaveControllers {
			width: 250px;
			font-size: 80%;
		}
		.editableSingle button, .editableSingle input {
			padding: 3px;
		}
		a.editFieldRemove {
			color: red;
		}
	</style> 
        
<div class="wrapper">
        <fieldset class="adminuser">
        <legend><?php echo $name?></legend>
        
            <table border="1" id="data" cellspacing="0" width="100%">            			
		<tr>
<?php
	     
        	   foreach ($columns as $key=>$value) {
          	    if($key == "id" || $key == "userid") continue;
                    echo "<th>$key</th>";                
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
	<div align="right"><button id="create-<?php echo $dval?>">Create new record</button></div>          
        </fieldset>                
</div>        

<?php if($dval == "user"):?>
<div id="createuser-form" title="Create new user"> 
	<p class="validateTips">All form fields are required.</p> 
 
	<form action="homer.php" name="createuser" id="createuser">
	<fieldset> 		
		<label for="email">Email/ID</label> 
		<input type="text" name="email" id="email" value="" class="text ui-widget-content ui-corner-all" /> 
		<label for="password">Password</label> 
		<input type="password" name="password" id="password" value="" class="text ui-widget-content ui-corner-all" /> 
		<label for="name">Level</label> 
		<input type="text" name="level" id="level" class="text ui-widget-content ui-corner-all" value="1" /> 
	</fieldset> 
	<input type="hidden" name="task" value="createuser">
	<input type="hidden" name="returntask" value="<?php echo $task;?>">
	</form> 
</div> 
<?php endif;?>

<?php if($dval == "host"):?>
<div id="createhost-form" title="Create new host"> 
	<form action="homer.php" name="createhost" id="createhost">
	<fieldset> 		
		<label for="email">Host</label> 
		<input type="text" name="host" id="host" value="" class="text ui-widget-content ui-corner-all" /> 
		<label for="name">Name</label> 
		<input type="text" name="name" id="name" class="text ui-widget-content ui-corner-all" /> 
		<label for="status">Status</label> 
		<input type="text" name="status" id="status" class="text ui-widget-content ui-corner-all" value="1" /> 
	</fieldset> 
	<input type="hidden" name="task" value="createhost">
	<input type="hidden" name="returntask" value="<?php echo $task;?>">
	</form> 
</div> 
<?php endif;?>

<?php if($dval == "node"):?>
<div id="createnode-form" title="Create new node"> 
	<form action="homer.php" name="createnode" id="createnode">
	<fieldset> 		
		<label for="email">Host</label> 
		<input type="text" name="host" id="host" value="" class="text ui-widget-content ui-corner-all" /> 
		<label for="name">Name</label> 
		<input type="text" name="name" id="name" class="text ui-widget-content ui-corner-all" /> 
		<label for="status">Status</label> 
		<input type="text" name="status" id="status" class="text ui-widget-content ui-corner-all" value="1"/> 
	</fieldset> 
	<input type="hidden" name="task" value="createnode">
	<input type="hidden" name="returntask" value="<?php echo $task;?>">
	</form> 
</div> 
<?php endif;?>



<?php
        }


        function displayStats() {
?>

<div class="wrapper">
        <div id="Modules"></div>

	<script type="text/javascript">
        jQuery(document).ready( function($) {

<?php

        // Scan Modules directory and display
        $submodules = array_filter(glob('modules/*'), 'is_dir');
        $modcount = 0;
        foreach( $submodules as $key => $value){
?>

                $('#Modules').append('<iframe id="stats<?php echo $modcount ?>" frameborder="0" scrolling="no" style="width:100%; height:315px;" />');
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

}

?>


