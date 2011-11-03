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

?>

<?php 

class HTML_ToolBox {


	function displayToolBox() {
?>

<!--    </form> -->
	<script type="text/javascript" src="js/cookie.jquery.js"></script>
	<script type="text/javascript" src="js/inettuts3.js"></script> 
        <script type="text/javascript" src="js/jquery.Dialog.js"></script>
    	<script src="js/jquery.timeentry.js" type="text/javascript"></script>
        <script type="text/javascript">
            $(function(){
              $('#from_date').datepicker({ dateFormat: 'dd-mm-yy' });
		          $('#to_date').datepicker({ dateFormat: 'dd-mm-yy' });
	 	         $('.timepicker1').timeEntry({show24Hours: true, showSeconds: true});
            	$('.timepicker2').timeEntry({show24Hours: true, showSeconds: true});

             iNettuts.init();
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
	<center>

        <ul id="column1" class="column" style="width: 11%;">
		<br>

<!--
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
				<input type="button" style="background: transparent;" title="Kill SIPVICIOUS" onclick="killVic();" value="Crash SIPVICIOUS" role="button"  class="ui-button ui-widget ui-state-default ui-corner-all">
				</td></tr>
			</table>
			<br>
                   </div>
            </li>
-->
<?php
        // dynamic modules
        if (MODULES != 0) {
	echo "<script type=\"text/javascript\" src=\"js/highstock.js\"></script>";

        // Scan Modules directory and display
        $submodules = array_filter(glob('modules/*'), 'is_dir');
        $modcount = 0;
          foreach( $submodules as $key => $value){
?>
           <li class="widget color-yellow" id="dyn-widget<?php echo $key ?>">
                <div class="widget-head">
                    <h3><?php echo $value ?></h3>
                </div>
                <div class="widget-content">
                <?php include($value."/index_dyn.php"); ?>
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

  	<li class="widget color-green" id="widget-last">
                <div class="widget-head"><h3>Last SIP</h3></div>
                <div class="widget-content">
                <div id="live"></div><br>
                <div id="live-control">
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
                          $.getJSON("api/api.php?task=last&limit=5",function(data)
                          {
                          $('#live').html('');
                          $.each(data.last, function(i,indata)
                        	{
				var ddt = indata.date.split(" ");
                        	var div_data =
              			"<p align=left>"+ddt[1]+" <a href=javascript:popMessage('"+indata.id+"');>"+indata.method+
                        	"</a> from: <b>"+indata.from_user+"</b> to: <b>"+indata.to_user+"</b></p>";
                        	$(div_data).appendTo("#live");
                        	});
                          });
                        return false;
                        });

			$(document).ready(function()
                        {
				
   			 $('#timer1').change(function () { clearInterval(last_refresh); setTT(this.value); }); 

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

<!-- last 5 calls via API -->

        <li class="widget color-green" id="widget-calls">
                <div class="widget-head"><h3>Last Calls</h3></div>
                <div class="widget-content">
                <div id="livecalls"></div><br>
                <div id="calls-control">
			<!--	<input type='number' size='2' value=''  style="width: 20; border: 0; background: #fff; float: right;  margin-left: 5; height: 15;" class="ui-corner-all ui-widget2"> -->
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
                        $.getJSON("api/api.php?task=search&field=METHOD&value=INVITE&limit=5",function(data)
                        {
                        $('#livecalls').html('');
                        $.each(data.session, function(i,indata)
                        {
			var ddt = indata.date.split(" ");
                        var div_data =
                        "<p align=left>"+ddt[1]+" <a href=javascript:popMessage('"+indata.id+"');>"+indata.method+
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
				        <td width="150" class="tablerow_two paramlist_key"><label for="date" title=".">From Date</label></td>
				        <td class="tablerow_two">
				        <input size="11" type="text" id="from_date"  class="textfieldstyle2" name="from_date" value="<?php if(isset($search['from_date'])) echo $search['from_date']; else echo date("d-m-Y");?> ">
				        &nbsp;-&nbsp;
				        <input type="text" name="from_time" id="from_time" class="textfieldstyle2 timepicker1" size="6" value="<?php if(isset($search['from_time'])) echo $search['from_time']; else echo $ft;?>" /> 
				        </td>
				        </tr>
				        <tr>
				        <td width="40%" class="tablerow_two paramlist_key"><label for="time" title=".">To Date</label></td>
				        <td class="tablerow_two">
				        <input size="11" type="text" id="to_date"  class="textfieldstyle2" name="to_date" value="<?php if(isset($search['to_date'])) echo $search['to_date']; else echo date("d-m-Y");?> ">
				        &nbsp;-&nbsp;
				        <input type="text" name="to_time" id="to_time" class="textfieldstyle2 timepicker2" size="6" value="<?php if(isset($search['to_time'])) echo $search['to_time']; else echo $tt; ?>" />
				        </td>


					</tr>
					<tr>
                                        <td width="150" class="tablerow_two">
                                                <label for="pcap_session" title="pcap_session">MATCH:</label>
                                        </td>
					 <td>
                                                <select name="pcap_match" id="pcap_match" value="" />
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
			
				<input type="button" style="background: transparent;" title="Generate PCAP" onclick="if($('#pcap_session').val() != ''){window.open('pcap.php?'+$('#pcap_match').val()+'='+$('#pcap_session').val()+'&from_date='+$('#from_date').val()+'&to_date='+$('#to_date').val()+'&from_time='+$('#from_time').val()+'&to_time='+$('#to_time').val() );} else {alert('no '+$('#pcap_match').val()+'!');}" value="Generate PCAP" role="button"  class="ui-button ui-widget ui-state-default ui-corner-all">
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
                                                <label for="phpsip_from" title="From">From</label>
                                        </td>
                                        <td>
                                                <input type="text" name="phpsip_from" id="phpsip_from" class="textfieldstyle2" size="40" value="user@domain.com" />

                                        </td>
                                </tr>
                                <tr>
                                        <td width="150" class="tablerow_two">
                                                <label for="phpsip_to" title="to">To</label>
                                        </td>
                                        <td>
                                                <input type="text" name="phpsip_to" id="phpsip_to" class="textfieldstyle2" size="40" value="user@domain.com" />

                                        </td>
                                </tr>
                                <tr>
                                        <td width="150" class="tablerow_two">
                                                <label for="phpsip_from" title="From">Proxy/Gateway</label>
                                        </td>
                                        <td>
                                                <input type="text" name="phpsip_prox" id="phpsip_prox" class="textfieldstyle2" size="40" value="proxy.domain.com:5060" />

                                        </td>
                                </tr>
                                <tr>
                                        <td width="150" class="tablerow_two">
                                                <label for="phpsip_from" title="From">Header Data</label>
                                        </td>
                                        <td>
                                                <input type="text" name="phpsip_head" id="phpsip_head" class="textfieldstyle2" size="40" placeholder="Optional" />

                                        </td>
                                </tr>
				<tr>
					<td>	</td>
                                         <td>
                                                <select name="phpsip_meth" id="phpsip_meth" value="" />
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




