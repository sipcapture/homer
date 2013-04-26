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

class HTML_Admin {

	function displayNewAdminOverView($allrows, $allnames, $task, $alldval) {

			global $mynodeshost, $task;
			        	
		
?>
	<script type="text/javascript" src="js/cookie.jquery.js"></script>
	<script type="text/javascript" src="js/inettuts3.js"></script> 
        <script type="text/javascript" src="js/jquery.Dialog.js"></script>
        <script type="text/javascript" src="js/jquery.inlineEdit.js"></script>
        <script type="text/javascript">
            $(function(){

		$('#date').datepicker({ dateFormat: 'dd-mm-yy' });
	
		 iNettuts.init();

                $.inlineEdit({
			categoryhost: 'adminajax.php?type=host&categoryId=',			
			categoryname: 'adminajax.php?type=name&categoryId=',
			categorystatus: 'adminajax.php?type=status&categoryId=',
			categorydbport: 'adminajax.php?type=dbport&categoryId=',
			categorydbname: 'adminajax.php?type=dbname&categoryId=',
			categorydbusername: 'adminajax.php?type=dbusername&categoryId=',
			categorydbpassword: 'adminajax.php?type=dbpassword&categoryId=',
			categorydbtables: 'adminajax.php?type=dbtables&categoryId=',
			categorypassword: 'adminajax.php?type=password&categoryId=',			
			categoryuseremail: 'adminajax.php?type=useremail&categoryId=',
			categoryuserlevel: 'adminajax.php?type=userlevel&categoryId=',			
			remove: 'adminajax.php?type=remove&categoryId='
		}, {
	
			animate: false,
	
			filterElementValue: function($o){
                                return $o.html();
			},
	
			afterSave: function(o){
        if(o.type == 'categorypassword') {
               $('.categorypassword.id' + o.id).html('xxx');
        }
				if (o.type == 'category2name') {
					$('.category2name.id' + o.id).prepend('$');
			}
		}	            
                });
                                                    
            });
        
        </script>
        <style type="text/css"> 
		#data td {
			width: 1px;
			vertical-align: top;
			cursor: pointer;
		}
		.editFieldSaveControllers {
			width: 150px;
			font-size: 80%;
		}
		.editableSingle button, .editableSingle input {
			padding: 1px;
		}

		.editableDropDown button, .editableDropDown input {
			padding: 1px;
		}		
		a.editFieldRemove {
			color: red;
		}
		a.editFieldCancel {
			color: orange;
		}
	</style> 
        
<!-- admin mod start -->
  <div id="columns"  style="margin: 1px 1px 0 1px;">
	<center>

        <ul id="column1" class="column" style="width: 9%;">
		<br>


<!-- start db tools -->
<?php

	}
	
	function displayAdminUsers($datas,$names,$types) {
	
	/* USERS/HOSTS/NODES  FORM */
        $headers  = array("USERS","ALIASES","DB NODES"); 
        $adminGroup = array("Admin","Manager","User","Guest");
                
        foreach($datas as $index=>$rows) {

		$name=$names[$index];
		$type = $types[$index];
		/* HEADER */
		$header = $headers[$index];
		$columns = $rows[0];		
    	    	    
?>	
      	    <li class="widget color-orange" id="widget-admin<?php echo $type;?>">
        	    <div class="widget-head">
	                    <h3><?php echo $header; ?></h3>
        	        </div>
	                <div class="widget-content">
			<br>
            <table border="1" id="data" cellspacing="0" width="95%" style="background: #f9f9f9;">            	
		<tr>
<?php
	     
        	   foreach ($columns as $key=>$value) {
          	    if($key == "id" || $key == "userid") continue;
          	    if($key == "status" || $key == "dbport" ) { $cwid="50px";} 
          	    else if($key == "password" || $key == "useremail") { $cwid="250px";} 
		    else {$cwid="auto";}
			$ktitle = strtoupper($key);
                    echo "<th style=\"width:".$cwid.";background:#c2c2c2;\">$ktitle</th>";                
                }        
		echo "</tr>";

                foreach($rows as $row) {      
                    
                    echo "<tr align='center'>\n";

                    foreach($row as $key=>$value) {
                        if($key == "id" || $key == "userid") continue;          
                        $id = !isset($row->userid) ? $row->id : $row->userid;
                        
                        if($key == "password") $value = "******";
                        
                        if($key == "userlevel") 
                        	echo "<td class=\"editableDropDown category{$key} removable id{$type}{$id}\">".$adminGroup[$value-1]."</td>\n";                  
                        else echo "<td class=\"editableSingle category{$key} removable id{$type}{$id}\">$value</td>\n";                    
                    }
                    echo "</tr>\n";
}
?>			    
	            </table>               
			<br><div id="bar_<? echo $name ?>" align="right"><button id="create-<?php echo $type?>">Create New</button></div><br>
		</div>
		</li>
<?php 
	}
?>	
<!-- end db tools -->


	</ul>

<?php
	}
	
	function displayAdminInfo() {
	
?>	


<!-- column2 start -->

        <ul id="column2" class="column" style="margin: 0 0 0 0; min-height: 0px; height: 0px; width: 80%" >
       

	<!-- about widget -->

	<li class="widget color-blue" id="widget-about">
                <div class="widget-head"><h3>About</h3></div>
                <div class="widget-content">
		<table width=100%><tr><td><center>
		<br><h1><font size=+2>webHomer</font> <?php echo WEBHOMER_VERSION;?> </h1>
		<br>
		AGPLv3 OSS License, Copyright 2011-2013 SIPCapture Labs<br>
		Please register at <a href="http://sipcapture.org" target="_blank">http://www.sipcapture.org</a><br>
		<br></center>
		</td><td><br>
		<div style="margin-left:2em">
		- <a href="http://homer.googlecode.com" target="_blank">Homer GIT Repository</a><br>
		- <a href="https://code.google.com/p/homer/wiki/FAQ" target="_blank">Frequently Asked Questions</a><br>
		- <a href="https://code.google.com/p/homer/issues/list" target="_blank">Open Issues/Tickets</a><br>
		- <a href="https://www.paypal.com/cgi-bin/webscr?cmd=_donations&business=donation%40sipcapture%2eorg&lc=US&item_name=SIPCAPTURE&no_note=0&currency_code=EUR&bn=PP%2dDonationsBF%3abtn_donateCC_LG%2egif%3aNonHostedGuest" target="_blank">Donate to Homer Project</a><br>
		</div></td></tr></table>

 		</div>
	</li>
<!-- system/stats tab -->

	<li  class="widget color-green" id="widget-prefs">
		<div class="widget-head"><h3>License Status</h3></div>
                <div class="widget-content"><br>
		<br><h1>SERVER LICENSE: FREE/UNSUPPORTED</h1><br>
		Please obtain a FREE license at <a href='http://sipcapture.org'>sipcapture.org</a><br>

<?php


// Check for new definitions in configuration_example
if (NOCHECK != 1) {

	if (!is_writable(PCAPDIR)) {
                echo "<b>WARNING: ".PCAPDIR." MUST be writable!</b><br>";
                echo "<br><hr><br>";
        }

 }
?>

	<br>

<!--

	<table border="0" id="prefs" cellspacing="0" width="95%" style="background: transparent;">
   


<?php

// Print subset
$userdef = get_defined_constants(true);
        foreach($userdef['user'] as $key => $value){
	if(!preg_match("/HOMER_|__|_HOMER|RADIUS_|DB|USER|IERROR|PW|ACCESS_|CSHARK_|GEOIP_URL/", "$key")){
        echo '<tr><td width="30%">'.$key.'</td><td>';
	echo '<button class="ui-state-default ui-button ui-widget ui-corner-all" style="width:400px;" disabled>';
	if ($value != '0') {echo $value;} else {echo "<i>(default)</i>";}
	echo "</button>";
	echo '</td></tr>';
	}
        }
?>



	</table><br>
-->

	</div>
	</li>

<!--
	</ul>

	<ul id="column3" class="column" style="margin: 0 0 0 0; min-height: 0px; height: 0px; width: 40%">

-->
		
<?php
	}
	
	function displayAdminHealth($report) {

	if (SERVICE_MONITOR != 0) {

?>	
	


		 <li class="widget color-green" id="widget-alarms">
                <div class="widget-head">
                    <h3>Server Health</h3>
                </div>
                <div class="widget-content">
		<br><h1>Homer Core</h1><br><br>

		<table  class="bodystyle" cellspacing="0" width="95%" height="132">

<?php
	 foreach ($report as $key=>$value) {

?>
		  <tr>
			 <td><?php echo $key ?></td>
		    <td>
<!--			<input type="button" value="<?php echo $value ? "SERVICE OK" : "SERVICE KO"; ?>"  style="background: transparent;" role="button"  role="button"  class="<?php echo $value ? " ui-state-default" : " ui-state-error"; ?> ui-button ui-widget ui-corner-all" disabled> --> 
		<button id='sw_autocomplete' style='width:150;' class=' class="<?php echo $value ? " ui-state-default" : " ui-state-error" ?> ui-button ui-widget ui-corner-all'><?php echo $value ? "SERVICE OK" : "SERVICE KO"; ?></button>
		    </td>
		  </tr>

<?php
	}
?>
		
		</table><br>
		</div>
		</li>

<?php 
		foreach($report as $key=>$value) {
			if ($value != 1) $alarm=1; 
		}
		if ($alarm) { 
?>
		<script type="text/javascript">
                        jQuery('#widget-alarms').removeClass("color-green").addClass("color-red");
                </script>
<?php 
		} 

	} 

	}

 function displayNetworkStats($bwstats) {

	if (ADMIN_NETSTAT != 0) {	
?>

<!-- Netstats -->
	 <li class="widget color-blue" id="widget-network">
                <div class="widget-head"><h3>Network</h3></div>
                <div class="widget-content">

                <br><h1>Network Status</h1>
	  <br><br>
	  <table  class="bodystyle" cellspacing="0" width="95%">
	  
<?php
 foreach ($bwstats as $key=>$value) {

	echo "<tr><td>".$key."</td><td>".$value."</td></tr>";
}
?>
	</table><br>
                </div>
        </li>



<?php
	}
  }

 function displayDBStats($dbstats) {

	if (ADMIN_DBSTAT != 0) {
?>

<!-- DBstats -->
	 <li class="widget color-blue" id="widget-database">
                <div class="widget-head"><h3>Database</h3></div>
                <div class="widget-content">

                <br><h1>Database Status</h1>
	  <br><br>
	  <table  class="bodystyle" cellspacing="0" width="95%">
	  
<?php
 foreach ($dbstats as $key=>$value) {

	echo "<tr><td>".$key."</td><td>".$value."</td></tr>";
}
?>
	</table><br>
                </div>
        </li>



<?php
	}
   }

	function displayAdminForms() {
	
?>	
		

<!-- HORIZONTAL CONTAINER - NOT IN USE
<ul id="column04" class="column"  style="width: 81%; height: 10; margin: 0 0 0 90px ;">
</ul>
-->


<!-- </div> -->

<div id="createuser-form" title="Create new user"> 
	<p class="validateTips" style="margin:10px;">
	New HOMER Account<br>
	All form fields are required.
	</p> 
 	<br>
	<form action="index.php" name="createuser" id="createuser">
	<fieldset> 		
		<label style="width:40%;display:block;float:left;" for="email">Email/ID &nbsp;</label> 
		<input type="text" name="email" id="email" value="" class="text ui-widget-content ui-corner-all" /><br> 
		<label style="width:40%;display:block;float:left;" for="password">Password</label> 
		<input type="password" name="password" id="password" value="" class="text ui-widget-content ui-corner-all" /><br> 
		<label style="width:40%;display:block;float:left;" for="name">UserLevel</label> 
		<select name=level" id="level" class="text ui-widget-content ui-corner-all">
			<option value="1">Admin</option>
			<option value="2">Manager</option>
			<option value="3">User</option>
			<option value="4">Guest</option>			
		</select>
	</fieldset> 
	<input type="hidden" name="task" value="createuser">
	<input type="hidden" name="component" value="admin">
	<input type="hidden" name="returntask" value="<?php if (isset($task)) echo $task;?>">
	</form> 
</div> 

<div id="createhost-form" title="Create new host alias"> 
	<form action="index.php" name="createhost" id="createhost">
	<fieldset> 		
		<label style="width:40%;display:block;float:left;" for="email">Host IP</label> 
		<input type="text" name="host" id="host" value="" class="text ui-widget-content ui-corner-all" /><br> 
		<label style="width:40%;display:block;float:left;" for="name">Host Name</label> 
		<input type="text" name="name" id="name" class="text ui-widget-content ui-corner-all" /> <br>
		<label style="width:40%;display:block;float:left;" for="status">Status</label>
		<select name="status" id="status" class="text ui-widget-content ui-corner-all" selected="1">
  			<option value="1">Active</option>
  			<option value="0">Disabled</option>
		</select> 
	</fieldset> 
	<input type="hidden" name="task" value="createhost">
	<input type="hidden" name="component" value="admin">
	<input type="hidden" name="returntask" value="<?php if (isset($task)) echo $task;?>">
	</form> 
</div> 

<div id="createnode-form" title="Create new node"> 
	<form action="index.php" name="createnode" id="createnode">
	<fieldset> 		
		<label style="width:40%;display:block;float:left;" for="email">Host</label> 
		<input type="text" name="host" id="host" value="" class="text ui-widget-content ui-corner-all" /> <br>
		<label style="width:40%;display:block;float:left;" for="name">Name</label> 
		<input type="text" name="name" id="name" class="text ui-widget-content ui-corner-all" /> <br>
		<label style="width:40%;display:block;float:left;" for="status">Status</label>
		<select name="status" id="status" class="text ui-widget-content ui-corner-all" selected="1">
  			<option value="1">Active</option>
  			<option value="0">Disabled</option>
		</select>  <br>
		<label style="width:40%;display:block;float:left;" for="name">Database</label> 
		<input type="text" name="dbname" id="dbname" class="text ui-widget-content ui-corner-all" /> <br>
		<label style="width:40%;display:block;float:left;" for="name">Database Port</label> 
		<input type="text" name="dbport" id="dbport" class="text ui-widget-content ui-corner-all" /> <br>
		<label style="width:40%;display:block;float:left;" for="name">Database User</label> 
		<input type="text" name="dbusername" id="dbusername" class="text ui-widget-content ui-corner-all" /> <br>
		<label style="width:40%;display:block;float:left;" for="name">Database Password</label> 
		<input type="text" name="dbtables" id="dbtables" class="text ui-widget-content ui-corner-all" /> <br>
		<label style="width:40%;display:block;float:left;" for="name">Database Tables</label> 
		<input type="text" name="dbpassword" id="dbpassword" class="text ui-widget-content ui-corner-all" /> <br>
	</fieldset> 
	<input type="hidden" name="task" value="createnode">
	<input type="hidden" name="component" value="admin">
	<input type="hidden" name="returntask" value="<?php echo $task;?>">
	</form> 
</div> 

<!-- admin mod end -->               
<?php

	}
}

?>
