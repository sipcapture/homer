<?php
/*
 * HOMER Web Interface
 * App: Homer's Stats generator (Alternative Version)
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

if (!defined('APILOC')) {
$included = 1;
include('../../configuration.php');
} else { $included = 0; }


date_default_timezone_set(CFLOW_TIMEZONE);
$offset = STAT_OFFSET;
$hours = STAT_RANGE;

$to_date = date("Y-m-d H:i:s", time() );
$from_date = date("Y-m-d H:i:s", time() - ( $hours * 3600 ) );

?>

                <script type="text/javascript" language="javascript" src="js/jquery.dataTables.min.js"></script>        
		<div id="trap" style="width: 99%;">
        		<div id="chart11" style="min-width: 360px; width: 99%; margin-left: 1px; float: center; height: 220px">
    	            	    <textarea style="min-width: 360px; width: 99%; margin-left: 1px; float: center; height: 220px" readonly id='alarmtext'></textarea>
        		</div>				
                </div>
 
<script type="text/javascript">

// $ = jQuery;

var oTable;
var refreshIntervalId;

var ftime = '<?php echo $from_date; ?>';
var ttime = '<?php echo $to_date; ?>';
var result;


function getAlarmData(ft,tt,method,url,cseq) {

        var sendData;
                var mydata = {
                   method: method,
                   cseq: cseq,
                   from_datetime: ft,
                   to_datetime: tt
                };

                if(mydata != null) sendData = 'data=' + $.toJSON(mydata);
                var temp = [];
                var qos = {};

		console.log("QOS: "+sendData);

                // Send
                $.ajax({
                    url: url,
                    type: 'POST',
                    async: false,
                    dataType: 'json',
                    data: sendData,
                    success: function (msg,code) {
                                //Success
                            if(msg.status == 'ok') {
                                // console.log(msg.data);
                                result = msg.data;
                                $.each(msg.data, function(i,item){
					var date = new Date(item.from_date);
                                        var mss = date.getTime() / 1000;

					if (item.type == 'asr') {
	                                        qos["asr"] = item.result;
						console.log(item.result);
					} else if (item.type == 'ner') {
        	                                qos["ner"] = item.result;
						console.log(item.result);
					} else if (item.method == 'INVITE') {
						if (item.auth == 0 ) {
							qos["invite"] = item.total;
						}
					} else if (item.method == '200' || item.method == '401') {
                                                        qos["subvite"] = item.total;
					} else if (item.method == 'REGISTER') {
						if (item.auth == 0 ) {
							qos["authfail"] = item.total;
						} else {
							qos["auth"] = item.total;

						}
					}
                                });
                            }
                            else {
				console.log('ERROR:' +msg.status);
                                // alert("Error: " + msg.status);
                            }
                    },  //Error
                    error: function (xhr, str) {
                          // alert("Error");
                    }
                });

                // return temp;
                return qos;
}


function updateAlarm(status,id) {

                var sendData;
                var mydata = {
                   status: status,
                   id: id
                };
                
                var url = "<?php echo APILOC;?>alarm/update";

                if(mydata != null) sendData = 'data=' + $.toJSON(mydata);

                // Send
                $.ajax({
                    url: url,
                    type: 'POST',
                    async: false,
                    dataType: 'json',
                    data: sendData,
                    success: function (msg,code) {
                                //Success
                            if(msg.status == 'ok') {	
                                // console.log(msg.data);
                                //oTable.fnReloadAjax();                                                                                                
                            }
                            else {
				console.log('ERROR:' +msg.status);
                                // alert("Error: " + msg.status);
                            }
                    },  //Error
                    error: function (xhr, str) {
                          // alert("Error");
                    }
                });

                // return temp;
                return true;
}


function loadAlarmData() {

        var sendData;
	var url = '<?php echo APILOC;?>alarm/data/short';

	//if(mydata != null) sendData = '?data=' + $.toJSON(mydata);        
	// Send
	var alarm;

        $.ajax({
                    url: url,
                    type: 'POST',
                    async: false,
                    dataType: 'json',
                    data: sendData,
                    success: function (msg,code) {
                                //Success

                            if(msg.status == 'ok') {
                                console.log(msg.data);
                                result = msg.data;
                                var html = "";
                                
                                $.each(msg.data, function(i,item){
                                        console.log(i);
                                        html += '[#'+item.id+'] ' + item.create_date + ': "' + item.type + '" Count:' + item.total 
					     + '\n[>'+item.id+'] IP:' +item.source_ip + ' ' + item.description + ' \n';
					console.log(item.create_date);
                                });
                                
                                $('#alarmtext').val(html);
                                
                            }
                            else {
				console.log('ERROR:' +msg.status);
                                // alert("Error: " + msg.status);
                            }
                    },  //Error
                    error: function (xhr, str) {
                          // alert("Error");
                    }
        });
}

jQuery(document).ready(function($) {

	// get new API charts
	loadAlarmData();
});


</script>		

