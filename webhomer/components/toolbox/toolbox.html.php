<?php
/*
 * HOMER Web Interface
 * Homer's Toolbox Sub
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


class HTML_ToolBox {


	static function displayToolBox() {

?>

<!--    </form> -->
	<script type="text/javascript" src="js/cookie.jquery.js"></script>
	<script type="text/javascript" src="js/inettuts3.js"></script> 
        <script type="text/javascript" src="js/jquery.Dialog.js"></script>
    	<script src="js/jquery.timeentry.js" type="text/javascript"></script>
        <script type="text/javascript">
	$(function()
                        {
             $(document).ready(function(){
              $('#from_date').datepicker({ dateFormat: 'dd-mm-yy' });
		          $('#to_date').datepicker({ dateFormat: 'dd-mm-yy' });
	 	         $('.timepicker1').timeEntry({show24Hours: true, showSeconds: true});
            	$('.timepicker2').timeEntry({show24Hours: true, showSeconds: true});

             	iNettuts.init();
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
        
<!- admin mod start -->
  <div id="columns">

        <ul id="column1" class="column" style="width: 11%;">
		<br>


            <li class="widget color-orange" id="widget-call">  
                <div class="widget-head">
                    <h3>Kill-Vicious</h3>
                </div>
                <div class="widget-content"><br>
                   <table class="bodystyle" cellspacing="1"  height="50">
			 	<tr>
					<td width="150" class="tablerow_two">
						<label for="callid" title="Callid">Offender IP:</label>
					</td>
					<td>
						<input type="text" name="vicious_ip" id="vicious_ip" class="textfieldstyle2" size="40" value="" />
					</td>
					</tr><tr>
					 <td width="150" class="tablerow_two">
                                                <label for="callid" title="Callid">Offender Port:</label>
                                        </td>
					 <td>
						<input type="text" name="vicious_port" id="vicious_port" class="textfieldstyle2" size="6" value="" />
                                        </td>

				</tr>
				<tr>
				 <td width="50" class="tablerow_two">
                                        </td>
				<td>
				<br>
				<input type="button" style="background: transparent;" title="Kill SIPVICIOUS" onclick="sipKillVic();" value="Crash SIPVICIOUS" role="button"  class="ui-button ui-widget ui-state-default ui-corner-all">
				</td></tr>
			</table>
			<br>
                   </div>
            </li>

<?php
        // dynamic modules
        if (MODULES != 0 && IERROR != 1) {
		// Set chart engine
			$chart="data";
			echo "<script type=\"text/javascript\" src=\"js/jquery.flot.js\"></script>";
			echo "<script type=\"text/javascript\" src=\"js/jquery.flot.pie.js\"></script>";
			echo "<script type=\"text/javascript\" src=\"js/jquery.flot.threshold.js\"></script>";

        // Scan Modules directory and display
        $submodules = array_filter(glob('modules/*'), 'is_dir');
        $modcount = 0;
          foreach( $submodules as $key => $value){
?>
           <li class="widget color-yellow" id="dyn-widget<?php echo $key; ?>">
                <div class="widget-head">
                    <h3><?php echo $value; ?></h3>
                </div>
                <div class="widget-content">
                <?php include($value."/index_".$chart.".php"); ?>
                </div>
            </li>
<?php
          $modcount++;
          }

         }
?>

<!-- 
	<li class="widget color-red" id="widget-auth">
                <div class="widget-head"><h3>Auth Fail</h3></div>
                <div class="widget-content">
		<script type="text/javascript">
		$(function() 
			{
			$(document).ready(function()
			{
			$.getJSON("api/search/method/401&limit=4",function(data)
			// $.getJSON('api/message/all/last?data={"method":"401","cseq":"REGISTER"}',function(data)
			{
			$('#live401').html('');
			$.each(data.session, function(i,indata)
			{
			var div_data =
			"<p align=left>"+indata.date+" "+indata.method+
			" from: <b><a href=javascript:popMessage('"+indata.id+"'); >"+indata.from_user+"</a> "+indata.reply_reason+"</b> <b>"+indata.destination_ip+"</b></p>";
			$(div_data).appendTo("#live401");
			});
			}
			);
			return false;
			});
		 });
		</script>

		<div id="live401"></div><br>
 		</div>
	</li>
-->

	</ul>

<!-- column2 start -->

        <ul id="column2" class="column" >

<!--
  	<li class="widget color-green" id="widget-last">
                <div class="widget-head"><h3>Last SIP</h3></div>
                <div class="widget-content">
                <div id="live"></div><br>
                <div id="live-control">
		<button disabled id="refresh-list" style="width: 60; border: 0; background: #fff;  float: left; margin: 0 0 9 0;"  class="ui-button ui-widget2 ui-corner-all">results</button>  
		<select id="items1"  style="float: left; width: 45; border: 0;  margin-left: 5; height: 15;" >
			<option value="5">5</option>
			<option value="10">10</option>
			<option value="15">15</option>
			<option value="20">20</option>
		</select>
		<select id="timer1"  style="width: 45; border: 0; float: right;  margin-left: 5; height: 15;" >
			<option value="0">0</option>
			<option value="15">15</option>
			<option value="30">30</option>
			<option value="60">60</option>
		</select>
		<button id="refresh-last" style="width: 60; border: 0; background: #fff;  float: right; margin: 0 0 9 0;"  class="ui-button ui-widget2 ui-corner-all">refresh</button>  
		</div><br>
                <script type="text/javascript">
		 $('#refresh-last').click(function()
                          {
			  var itempool1 = $('#items1').val();
                          // $.getJSON("api/api.php?task=last_perf&limit="+itempool1,function(data)
                          $.getJSON('api/message/all/last?data={"limit":"'+itempool1+'"}',function(data)
                          {
                          $('#live').html('');
                          $.each(data.data, function(i,indata)
                        	{
				var ddt = indata.date.split(" ");

				var url = "utils.php?task=sipmessage&id="+indata.id+"&popuptype=<?php echo MESSAGE_POPUP;?>";
				url += "&from_time="+ddt[1]+"&from_date="+ddt[0]+"&tnode=<?php echo DEFAULTDBNODE ?>&tablename=<?php echo HOMER_TABLE; ?>";

				var furl = "cflow.php?cid[]="+indata.callid+
				"&from_time="+ddt[1]+"&to_time="+ddt[1]+"&from_date="+ddt[0]+"&to_date="+ddt[0]+
				<?php 
				 if (!defined('CFLOW_POPUP')) echo '"&popuptype=1"+';
				 else echo '"&popuptype='.CFLOW_POPUP.'"+';
				 // if (!defined('BLEGDETECT')) echo '""+'; 
				 // else echo '"&callid_bleg="+indata.callid+"'.BLEGCID.'"+';
		 		 // add location[]
                                 if(defined('DEFAULTDBNODE')) echo '"&location[]='.DEFAULTDBNODE.'"+';
				?>
				"";
				                           
                        	var div_data =
              			"<p align=left>"+ddt[1]+
			" [<a href=javascript:showCallFlow2(<?php echo MESSAGE_POPUP;?>,'"+indata.callid+"','"+furl+
			"');>#</a>] <a href=javascript:popMessage2(<?php echo MESSAGE_POPUP;?>,'"+escape(indata.id)+"','"+url+"');>"+indata.method+
			//        " <a href=javascript:popMessage2(<?php echo MESSAGE_POPUP;?>,'"+indata.id+"','"+url+"');>"+indata.method+
                        	"</a> from: <b>"+indata.from_user+"</b> to: <b>"+indata.to_user+"</b></p>";
                        	$(div_data).appendTo("#live");
                        	});
                          });
                        return false;
                        });

			$(document).ready(function()
                        {
				
   			 $('#timer1').change(function () { clearInterval(last_refresh); setTT(this.value); }); 
   			 $('#items1').change(function () { itempool1 = this.value;  $("#refresh-last").click(); }); 

			$("#refresh-last").click();
			var last_refresh = 0;
			function setTT(timer){
				if (timer == 0) { clearInterval(last_refresh); } else {
				var timerx = (timer*1000);
				last_refresh = setInterval(
				function ()
				{
				$('#refresh-last').click();
				}, timerx );
				//}, 10000);
				}
			}

			});
                </script>
		
                </div>
        </li>

        <li class="widget color-green" id="widget-calls">
                <div class="widget-head"><h3>Last Calls</h3></div>
                <div class="widget-content">
                <div id="livecalls"></div><br>
                <div id="calls-control">
		<button disabled id="refresh-list2" style="width: 60; border: 0; background: #fff;  float: left; margin: 0 0 9 0;"  class="ui-button ui-widget2 ui-corner-all">results</button>  
		<select id="items2"  style="float: left; width: 45; border: 0;  margin-left: 5; height: 15;" >
			<option value="5">5</option>
			<option value="10">10</option>
			<option value="15">15</option>
			<option value="20">20</option>
		</select>
		<select id="timer2"  style="width: 45; border: 0; float: right;  margin-left: 5; height: 15;" >
			<option value="0">0</option>
			<option value="15">15</option>
			<option value="30">30</option>
			<option value="60">60</option>
		</select>
		<button id="refresh-calls" style="width: 60; border: 0; background: #fff;  float: right; margin: 0 0 9 0;"  class="ui-button ui-widget2 ui-corner-all">refresh</button>  

		</div><br>		
                <script type="text/javascript">
                        $('#refresh-calls').click(function()
                        {
                        // $.getJSON("api/search/method/INVITE&limit=5",function(data)
			var itempool2 = $('#items2').val();
                        // $.getJSON("api/api.php?task=search&field=METHOD&value=INVITE&limit="+itempool2,function(data)
                        $.getJSON('api/message/all/last?data={"method":"INVITE","limit":"'+itempool2+'"}',function(data)
                        {
                        $('#livecalls').html('');
                        $.each(data.session, function(i,indata)
                        {

			var ddt = indata.date.split(" ");
			var diff=new Date();
			ddx = diff.getHours();			
			var url = "utils.php?task=sipmessage&id="+indata.id+"&popuptype=<?php echo MESSAGE_POPUP;?>";			
	 	        url += "&from_time="+ddt[1]+"&from_date="+ddt[0]+"&tnode=<?php echo DEFAULTDBNODE ?>&tablename=<?php echo HOMER_TABLE; ?>";

			var furl = "cflow.php?cid[]="+indata.callid+
			"&from_time="+ddt[1]+"&to_time="+ddt[1]+"&from_date="+ddt[0]+"&to_date="+ddt[0]+
			<?php 
			 if (!defined('CFLOW_POPUP')) echo '"&popuptype=1"+';
			 else echo '"&popuptype='.CFLOW_POPUP.'"+';
			  if (!defined('BLEGDETECT')) echo '""+'; 
			  else echo '"&callid_aleg="+indata.callid+"'.BLEGCID.'"+';
		 	  // add location[]
                          if(defined('DEFAULTDBNODE')) echo '"&location[]='.DEFAULTDBNODE.'"+';
			?>
			"";

                        var div_data =
                        "<p align=left>"+ddt[1]+ 
			" [<a href=javascript:showCallFlow2(<?php echo MESSAGE_POPUP;?>,'"+indata.callid+"','"+furl+
			"');>#</a>] <a href=javascript:popMessage2(<?php echo MESSAGE_POPUP;?>,'"+escape(indata.id)+"','"+url+"');>"+indata.method+
                        "</a> from: <b>"+indata.from_user+"</b> to: <b>"+indata.to_user+"</b></p>";
                        $(div_data).appendTo("#livecalls");
                        });
                        }
                        );
                        return false;
                        });

		 $(document).ready(function()
                 {
   			 $('#timer2').change(function () { clearInterval(calls_refresh); setTT(this.value); }); 
   			 $('#items2').change(function () { itempool2 = this.value; $("#refresh-calls").click(); }); 

			$("#refresh-calls").click();
			var calls_refresh = 0;
			function setTT(timer){
				if (timer == 0) { clearInterval(calls_refresh); } else {
				var timerx = (timer*1000);
				calls_refresh = setInterval(
				function ()
				{
				$('#refresh-calls').click();
				}, timerx );
				//}, 10000);
				}
			}


		 });

                </script>

                </div>
        </li>
-->

<!-- Freesearch via API -->

        <li class="widget color-green" id="widget-apitool">
                <div class="widget-head"><h3>SIP Filter #1</h3></div>
                <div class="widget-content">
                <div id="livetool"></div><br>
                <div id="tool-control">
<!--		<button disabled id="refresh-list3" style="width: 60; border: 0; background: #fff;  float: left; margin: 0 0 9 0;"  class="ui-button ui-widget2 ui-corner-all">results</button>  -->
		<select id="items3"  style="float: left; width: 45; border: 0;  margin-left: 5; height: 18;" >
			<option value="5">5</option>
			<option value="10" selected>10</option>
			<option value="15">15</option>
			<option value="20">20</option>
		</select>
		<select id="filter3"  style="float: left; width: 106; border: 0;  margin-left: 5; height: 18;" >
			<option value=""></option>
			<option value="ANY">(ANY)</option>
			<option value="INVITE">INVITE</option>
			<option value="REGISTER">REGISTER</option>
			<option value="OPTIONS">OPTIONS</option>
			<option value="BYE">BYE</option>
			<option value="SIP">SIP CODE -></option>
		</select>
		<input id="sipcode3" size="6" value=""  style="border: 1;  margin-left: 6; height: 18;" class="ui-corner-all ui-widget2">
		<select id="timer3"  style="width: 45; border: 0; float: right;  margin-left: 5; height: 18;" >
			<option value="0">0</option>
			<option value="15">15</option>
			<option value="30">30</option>
			<option value="60">60</option>
		</select>
		<button id="refresh-tool" style="width: 60; border: 0; background: #fff;  float: right; margin: 0 0 9 0;"  class="ui-button ui-widget2 ui-corner-all">refresh</button>  

		</div><br>		
                <script type="text/javascript">
			$('#sipcode3').hide();
                        $('#refresh-tool').click(function()
                        {
	
			var itempool3 = $('#items3').val();
			var filterpool3 = $('#filter3').val();
			if (filterpool3 == "SIP") {filterpool3=  $('#sipcode3').val(); }
			if (filterpool3 == "") {
				var apicall = '';
				$('#livetool').html('');
			} else if (filterpool3 == "ANY" ) {
                        	 // var apicall = "api/api.php?task=last_perf&limit="+itempool3;
                        	var apicall = 'api/message/all/last?data={"limit":"'+itempool3+'"}';
			} else {
                        	// var apicall = "api/api.php?task=search&field=METHOD&value="+filterpool3+"&limit="+itempool3;
				var apicall = 'api/message/all/last?data={"method":"'+filterpool3+'","limit":"'+itempool3+'"}';
                        }
			
                        //$.getJSON("api/api.php?task=search&field=METHOD&value="+filterpool3+"&limit="+itempool3,function(data)
                        $.getJSON(apicall,function(data)
			{
				console.log(data);

                        $('#livetool').html('');
			if (!data.session) {src=data.data} else {src=data.session}
                        //$.each(data.session, function(i,indata)
                        $.each(src, function(i,indata)
                        {

			var ddt = indata.date.split(" ");
			var diff=new Date();
			ddx = diff.getHours();			
			var url = "utils.php?task=sipmessage&id="+indata.id+"&popuptype=<?php echo MESSAGE_POPUP;?>";			
	 	        url += "&from_time="+ddt[1]+"&from_date="+ddt[0]+"&tnode=<?php echo DEFAULTDBNODE ?>&tablename=<?php echo HOMER_TABLE; ?>";

			var furl = "cflow.php?cid[]="+indata.callid+
			"&from_time="+ddt[1]+"&to_time="+ddt[1]+"&from_date="+ddt[0]+"&to_date="+ddt[0]+
			<?php 
			 if (!defined('CFLOW_POPUP')) echo '"&popuptype=1"+';
			 else echo '"&popuptype='.CFLOW_POPUP.'"+';
			  if (!defined('BLEGDETECT')) echo '""+'; 
			  else echo '"&callid_aleg="+indata.callid+"'.BLEGCID.'"+';
		          // add location[]
                          if(defined('DEFAULTDBNODE')) echo '"&location[]='.DEFAULTDBNODE.'"+';

			?>
			"";

                        var div_data =
                        "<p align=left>"+ddt[1]+ 
			" [<a href=javascript:showCallFlow2(<?php echo MESSAGE_POPUP;?>,'"+indata.callid+"','"+furl+
			"');>#</a>] <a href=javascript:popMessage2(<?php echo MESSAGE_POPUP;?>,'"+escape(indata.id)+"','"+url+"');>"+indata.method+
                        "</a> from: <b>"+indata.from_user+"</b> to: <b>"+indata.to_user+"</b></p>";
                        $(div_data).appendTo("#livetool");
                        });
                        }
                        );
                        return false;
                        });


		 $(document).ready(function()
                 {
   			 $('#timer3').change(function () { clearInterval(tool_refresh); setTT(this.value); }); 
   			 $('#items3').change(function () { itempool3 = this.value; $("#refresh-tool").click(); }); 
   			 $('#filter3').change(function () {
				if (this.value == "SIP") { 
					$('#sipcode3').show(); 
				} else { 
					$('#sipcode3').hide();  
					$("#refresh-tool").click();
				} 
			 }); 

			$("#refresh-tool").click();
			var tool_refresh = 0;
			function setTT(timer){
				if (timer == 0) { clearInterval(tool_refresh); } else {
				var timerx = (timer*1000);
				tool_refresh = setInterval(
				function ()
				{
				$('#refresh-tool').click();
				}, timerx );
				//}, 10000);
				}
			}

		 });

                </script>

                </div>
        </li>

<!-- Callsearch via API -->

        <li class="widget color-green" id="widget-apicall">
                <div class="widget-head"><h3>SIP Filter #2</h3></div>
                <div class="widget-content">
                <div id="livecalls"></div><br>
                <div id="call-control">
<!--		<button disabled id="refresh-list4" style="width: 60; border: 0; background: #fff;  float: left; margin: 0 0 9 0;"  class="ui-button ui-widget2 ui-corner-all">results</button>  -->
		<select id="items4"  style="float: left; width: 45; border: 0;  margin-left: 5; height: 18;" >
			<option value="5">5</option>
			<option value="10" selected>10</option>
			<option value="15">15</option>
			<option value="20">20</option>
		</select>
		<select id="filter4"  style="float: left; width: 50; border: 0;  margin-left: 5; height: 18;" >
			<option value=""></option>
			<option value="UID">UID</option>
			<option value="IP">IP</option>
		</select>
		<input id="sipcode4" size="15" value=""  style="border: 1;  margin-left: 6; height: 18;" class="ui-corner-all ui-widget2">
		<select id="timer4"  style="width: 45; border: 0; float: right;  margin-left: 5; height: 18;" >
			<option value="0">0</option>
			<option value="15">15</option>
			<option value="30">30</option>
			<option value="60">60</option>
		</select>
		<button id="apicall-tool" style="width: 60; border: 0; background: #fff;  float: right; margin: 0 0 9 0;"  class="ui-button ui-widget2 ui-corner-all">refresh</button>  

		</div><br>		
                <script type="text/javascript">
			$('#sipcode4').hide();
                        $('#apicall-tool').click(function()
                        {
	
			var itempool4 = $('#items4').val();
			var filterpool4 = $('#filter4').val();
			if (filterpool4 == "SIP") {
				filterpool4=  $('#sipcode4').val(); 
				var apicall = 'api/message/all/last?data={"method":"'+filterpool4+'","limit":"'+itempool4+'"}';
                        	// var apicall = "api/api.php?task=search&field=METHOD&value="+filterpool4+"&limit="+itempool4;
			} else if (filterpool4 == "IP") {
				filterpool4=  $('#sipcode4').val(); 
				var apicall = 'api/message/all/last?data={"source_ip":"'+filterpool4.toUpperCase()+'","limit":"'+itempool4+'"}';
                        	//var apicall = "api/api.php?task=last&ip="+filterpool4.toUpperCase()+"&limit="+itempool4;
			} else if (filterpool4 == "UID") {
				filterpool4=  $('#sipcode4').val(); 
				var apicall = 'api/message/all/last?data={"from_user":"'+filterpool4+'","limit":"'+itempool4+'"}';
                        	// var apicall = "api/api.php?task=last&user="+filterpool4+"&limit="+itempool4;
			} else if (filterpool4 == "") {
				var apicall = '';
				$('#livecalls').html('');
			} else if (filterpool4 == "ANY" ) {
                        	//var apicall = "api/api.php?task=last_perf&limit="+itempool4;
                        	var apicall = "api/message/all/last?data={'limit':'"+itempool4+"'}";
			} else {
                        	// var apicall = "api/api.php?task=search&field=METHOD&value="+filterpool4+"&limit="+itempool4;
				var apicall = 'api/message/all/last?data={"method":"'+filterpool4+'","limit":"'+itempool4+'"}';
                        }
			
                        //$.getJSON("api/api.php?task=search&field=METHOD&value="+filterpool4+"&limit="+itempool4,function(data)
                        $.getJSON(apicall,function(data)
			{
                        $('#livecalls').html('');
			if (!data.session) {src=data.data} else {src=data.session}
                        //$.each(data.session, function(i,indata)
                        $.each(src, function(i,indata)
                        {

			var ddt = indata.date.split(" ");
			var diff=new Date();
			ddx = diff.getHours();			
			var url = "utils.php?task=sipmessage&id="+indata.id+"&popuptype=<?php echo MESSAGE_POPUP;?>";			
	 	        url += "&from_time="+ddt[1]+"&from_date="+ddt[0]+"&tnode=<?php echo DEFAULTDBNODE ?>&tablename=<?php echo HOMER_TABLE; ?>";

			var furl = "cflow.php?cid[]="+indata.callid+
			"&from_time="+ddt[1]+"&to_time="+ddt[1]+"&from_date="+ddt[0]+"&to_date="+ddt[0]+
			<?php 
			 if (!defined('CFLOW_POPUP')) echo '"&popuptype=1"+';
			 else echo '"&popuptype='.CFLOW_POPUP.'"+';
			  if (!defined('BLEGDETECT')) echo '""+'; 
			  else echo '"&callid_aleg="+indata.callid+"'.BLEGCID.'"+';
		 	  // add location[]
                          if(defined('DEFAULTDBNODE')) echo '"&location[]='.DEFAULTDBNODE.'"+';

			?>
			"";

                        var div_data =
                        "<p align=left>"+ddt[1]+ 
			" [<a href=javascript:showCallFlow2(<?php echo MESSAGE_POPUP;?>,'"+indata.callid+"','"+furl+
			"');>#</a>] <a href=javascript:popMessage2(<?php echo MESSAGE_POPUP;?>,'"+escape(indata.id)+"','"+url+"');>"+indata.method+
                        "</a> from: <b>"+indata.from_user+"</b> to: <b>"+indata.to_user+"</b></p>";
                        $(div_data).appendTo("#livecalls");
                        });
                        }
                        );
                        return false;
                        });


		 $(document).ready(function()
                 {
   			 $('#timer4').change(function () { clearInterval(ctool_refresh); setTT(this.value); }); 
   			 $('#items4').change(function () { itempool4 = this.value; $("#apicall-tool").click(); }); 
   			 $('#filter4').change(function () {
				if (this.value == "SIP"|this.value=="UID"|this.value=="IP") {
					$('#sipcode4').val('');
					$('#sipcode4').show(); 
				} else { 
					$('#sipcode4').val('');
					$('#sipcode4').hide();  
					$("#apicall-tool").click();
				} 
			 }); 

			$("#apicall-tool").click();
			var ctool_refresh = 0;
			function setTT(timer){
				if (timer == 0) { clearInterval(ctool_refresh); } else {
				var timerx = (timer*1000);
				ctool_refresh = setInterval(
				function ()
				{
				$('#apicall-tool').click();
				}, timerx );
				//}, 10000);
				}
			}

		 });

                </script>

                </div>
        </li>

  	</ul>

 	<ul id="column3" class="column">

	<!-- PCAP GEN -->
 	<?php
                      $ft = date("H:i:s", strtotime("-1 hour"));
                      $tt = date("H:i:s");
        ?>

            <li class="widget color-blue" id="widget-pcap">  
                <div class="widget-head">
                    <h3>PCAP Factory</h3>
                </div>
                <div class="widget-content"><br>
                   <table class="bodystyle" cellspacing="1"  height="150" width="95%" >
			 	<tr>
				        <td width="150" class="tablerow_two paramlist_key"><label for="date" title="From this date">From Date</label></td>
				        <td class="tablerow_two">
				        <input size="11" type="text" id="from_date"  class="textfieldstyle2" name="from_date" value="<?php if(isset($search['from_date'])) echo $search['from_date']; else echo date("d-m-Y");?> ">
				        &nbsp;-&nbsp;
				        <input type="text" name="from_time" id="from_time" class="textfieldstyle2 timepicker1" size="6" value="<?php if(isset($search['from_time'])) echo $search['from_time']; else echo $ft;?>" /> 
				        </td>
				        </tr>
				        <tr>
				        <td width="40%" class="tablerow_two paramlist_key"><label for="time" title="Up to this date">To Date</label></td>
				        <td class="tablerow_two">
				        <input size="11" type="text" id="to_date"  class="textfieldstyle2" name="to_date" value="<?php if(isset($search['to_date'])) echo $search['to_date']; else echo date("d-m-Y");?> ">
				        &nbsp;-&nbsp;
				        <input type="text" name="to_time" id="to_time" class="textfieldstyle2 timepicker2" size="6" value="<?php if(isset($search['to_time'])) echo $search['to_time']; else echo $tt; ?>" />
				        </td>


					</tr>
					<tr>
                                        <td width="150" class="tablerow_two">
                                                <label for="pcap_session" title="PCAP Session">MATCH:</label>
                                        </td>
					 <td>
                                                <select name="pcap_match" id="pcap_match" value="" class="ui-select ui-widget ui-state-default ui-corner-all" />
                                                  <option value="cid" selected>Session-ID</option>
                                                  <option value="from_user">From User</option>
                                                  <option value="to_user">To User</option>
                                                </select>

					</td>
					</tr><tr>
					 <td width="150" class="paramlist_key">
                                        </td>

					<td>
                                                <input type="text" name="pcap_session" id="pcap_session" class="textfieldstyle2" size="40" />
                                        </td>
                                        </tr>

				<tr>
				 <td width="50" class="paramlist_key">
                                        </td>
				<td>
			
			       <input type="button" style="background: transparent;" title="Generate PCAP" onclick="if($('#pcap_session').val() != ''){window.open('pcap.php?'+$('#pcap_match').val()+'='+$('#pcap_session').val()+'&from_date='+$('#from_date').val()+'&to_date='+$('#to_date').val()+'&from_time='+$('#from_time').val()+'&to_time='+$('#to_time').val() );} else {alert('no '+$('#pcap_match').val()+'!');}" value="Generate PCAP" role="button"  class="ui-button ui-widget ui-state-default ui-corner-all"></input>
<?php if ( defined('PCAP_AGENT') && PCAP_AGENT != '' || defined('PCAP_AGENT4') && PCAP_AGENT4 != '' ) { ?>
			       <input type="button" style="background: transparent;" title="Import PCAP" onclick="adminAction('pcapin','','Import PCAP'); return false;" value="Import PCAP" role="button"  class="ui-button ui-widget ui-state-default ui-corner-all"></input>
<?php } ?>
			       </td></tr>
			</table>
			<br>
                   </div>
            </li>

	    <li class="widget color-blue" id="widget-sipsend">
                <div class="widget-head">
                    <h3>SIP Factory</h3>
                </div>
                <div class="widget-content"><br>
		 <table class="bodystyle" cellspacing="1"  height="150" width="95%">

                                <tr>
                                        <td width="150" class="tablerow_two">
                                                <label for="phpsip_from" title="From URI">From</label>
                                        </td>
                                        <td>
                                                <input type="text" name="phpsip_from" id="phpsip_from" class="textfieldstyle2" size="40" value="user@domain.com" />

                                        </td>
                                </tr>
                                <tr>
                                        <td width="150" class="tablerow_two">
                                                <label for="phpsip_to" title="To URI">To</label>
                                        </td>
                                        <td>
                                                <input type="text" name="phpsip_to" id="phpsip_to" class="textfieldstyle2" size="40" value="user@domain.com" />

                                        </td>
                                </tr>
                                <tr>
                                        <td width="150" class="tablerow_two">
                                                <label for="phpsip_from" title="Proxy URI">Proxy/Gateway</label>
                                        </td>
                                        <td>
                                                <input type="text" name="phpsip_prox" id="phpsip_prox" class="textfieldstyle2" size="40" value="proxy.domain.com:5060" />

                                        </td>
                                </tr>
                                <tr>
                                        <td width="150" class="tablerow_two">
                                                <label for="phpsip_from" title="Extra HEADER data">Header Data</label>
                                        </td>
                                        <td>
                                                <input type="text" name="phpsip_head" id="phpsip_head" class="textfieldstyle2" size="40" placeholder="Optional" />

                                        </td>
                                </tr>
				<tr>
					<td>	</td>
                                         <td>
                                                <select name="phpsip_meth" id="phpsip_meth" value="" class="ui-select ui-widget ui-state-default ui-corner-all"  />
                                                  <option value="OPTIONS">OPTIONS</option>
                                                  <option value="INVITE">INVITE</option>
                                                </select>
                                        </td>
                                </tr>
				 <tr>
					<td>	</td>
                                        <td>
	<input type="button" style="background: transparent;" title="Send SIP Message" onclick="sipSendForm();" value="Send SIP Message" role="button"  class="ui-button ui-widget ui-state-default ui-corner-all">

                                        </td>
				</tr>



                        </table>
		<br>

		</div>
	</li>



</ul>
<div>

<!-- admin mod end -->               
<?php
	}
}
?>




