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
		<div id="trap" style="width: 99%;">
		<div id="chart5" style="min-width: 90px; width: 32%; margin-left: 1px; float: left; height: 220px;"></div>
		<div id="chart4" style="min-width: 90px; width: 32%; margin-left: 1px; float: left; height: 220px;"></div>
	        <div id="chart6" style="min-width: 90px; width: 32%; margin-left: 1px; float: left; height: 220px;"></div>
		</div>
 
<div id="chart7" style="min-width: 360px; width: 99%; margin-left: 1px; float: center; height: 220px"></div>

<script type="text/javascript">

// $ = jQuery;



	// get new API charts

        var ftime = '<?php echo $from_date; ?>';
        var ttime = '<?php echo $to_date; ?>';
        var result;

        function getQoSData(ft, tt, method, url, cseq, auth, totag, t) {

                var sendData;
                var mydata = {
                   cseq: cseq,
                   from_datetime: ft,
                   to_datetime: tt
                };

		if(t == 0) {
		     mydata.method = method;
		     mydata.cseq = cseq;
                     mydata.totag = totag;
                     mydata.auth = auth;
                }
                else {
                     mydata.type = method;
                }

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
					} else if (item.method == '200' || item.method == '407') {
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

function loadQoSData(ftime, ttime) {

	var qos_1 = getQoSData(ftime,ttime, 'asr', '<?php echo APILOC;?>statistic/data/total', '', 0, 0, 1);
	var qos_2 = getQoSData(ftime,ttime, 'ner', '<?php echo APILOC;?>statistic/data/total', '', 0, 0, 1);
	var reg_qos = getQoSData(ftime,ttime,'REGISTER','<?php echo APILOC;?>statistic/method/total', '', 0, 0, 0);

	var call1_qos = getQoSData(ftime,ttime,'INVITE','<?php echo APILOC;?>statistic/method/total', '', 0, 0, 0);
	var call2_qos = getQoSData(ftime,ttime,'200','<?php echo APILOC;?>statistic/method/total','INVITE', 0, 0, 0);
	var call3_qos = getQoSData(ftime,ttime,'407','<?php echo APILOC;?>statistic/method/total','INVITE', 0, 0, 0);

	console.log('QoS stats');

	// asr + ner
	if (! qos_1["asr"] ) { qos_1["asr"] = 0; }
	if (! qos_2["ner"] ) { qos_2["ner"] = 0; }
	var asr1 = [[0, qos_1["asr"]]];
	var ner1 = [[1, qos_2["ner"]]];
	var asr1Display = qos_1["asr"];
	var ner1Display = qos_2["ner"];

	// registration
	var rok1 = [[0, reg_qos["auth"]]];
	var rko1 = [[1, reg_qos["authfail"]]];

	// invites
	if (! call3_qos["subvite"] ) { call3_qos["subvite"] = 0; }
	if (! call2_qos["subvite"] ) { call2_qos["subvite"] = 0; }
	if (! call1_qos["invite"] ) { call1_qos["invite"] = 0; }
	var calls_ok = parseInt(call2_qos["subvite"]);
	var calls_diff = parseInt(call1_qos["invite"]) - parseInt(call3_qos["subvite"]);
	var calls_ko = calls_ok - calls_diff;
	if(calls_ko < 0) calls_ko = 0;
	console.log(calls_ok + " - " + calls_ko);
	var cok1Display = calls_ok;
	var cok1 = [[0, calls_ok]];
	var cko1Display = calls_ko;
	var cko1 = [[1, calls_ko]];
	var cps = ((parseFloat(cok1Display)+parseFloat(cko1Display))/(3600*(<?php echo $hours ?>))).toFixed(3) ;


  	$.plot($("#chart5"),
                [
                { data: asr1, label: "ASR "+asr1Display,  bars: { show: true }, color: '#0cacfc' },
                { data: ner1, label: "NER "+ner1Display,  bars: { show: true }, color: '#0363f3' },
                ],
           {
               yaxes: [  { position: 'left', min: 0, max: 100 }                      ],
		xaxes: [ { ticks: 0 } ],

	 legend: {
                position: "sw",
                backgroundOpacity: 1
                },
	 bars:{
                  barWidth:0.9
            },    
	 grid: { 
  		borderWidth: 0 
		} 
          });

   	$.plot($("#chart4"),
                [
                { data: cok1, label: "Calls "+cok1Display,  bars: { show: true }, color: 'rgb(30, 180, 20)' },
                { data: cko1, label: "Failed "+cko1Display,  bars: { show: true }, color: 'rgb(200,20,30)' },
                { data: cps, label: "CPS "+cps,  bars: { show: false}, color: 'rgb(21,20,200)' },
                ],
           {
               yaxes: [  { position: 'left' }                    ],
		xaxes: [ { ticks: 0 } ],

	 legend: {
                position: "sw",
                backgroundOpacity: 1
                },
	  bars:{
                  barWidth:0.9
            },    
	  grid: {
                borderWidth: 0
                }


           });

   	$.plot($("#chart6"),
                [ 
                { data: rok1, label: "Register", bars: { show: true }, color: 'rgb(30, 80, 20)' },
                { data: rko1, label: "Failed",  bars: { show: true }, color: 'rgb(100, 10, 30)' },
                ],
           {
               yaxes: [  { position: 'left' }                    ],
                xaxes: [ { ticks: 0 } ],

	 legend: {
    		position: "sw",
    		backgroundOpacity: 1
  		},
	  bars:{
                  barWidth:0.9
            },    
	  grid: {
                borderWidth: 0
                }


           });

}

$(document).ready(function() {
   loadQoSData('<?php echo date("Y-m-d H:i:s", time() - 3600); ?>', '<?php echo date("Y-m-d H:i:s", time());?>');

});


</script>		

