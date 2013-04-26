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
//echo '<script type="text/javascript" src="js/jquery.flot.js"></script>';
} else { $included = 0; }


date_default_timezone_set(CFLOW_TIMEZONE);
$offset = STAT_OFFSET;
$xhours = STAT_RANGE;
// if(isset($_GET['range']) && intval($_GET['range']) <= 96 &&  intval($_GET['range']) >= 1) $xhours = intval($_GET['range']);
// if(isset($_GET['from_date']) && isset($_GET['to_date']) {
//	$from_date = intval($_GET['from_date']);
//	$from_date = intval($_GET['to_date']);
// } else {
	$to_date = date("Y-m-d H:i:s", time() );
	$from_date = date("Y-m-d H:i:s", time() - 3600 );
// }

?>

<div id="chart1" style="min-width: 360px; width: 99%; margin-left: 1px; float: center; height: 220px"></div>

<script type="text/javascript">

	// flot popup

	$.fn.UseTooltip = function () {
    	var previousPoint = null;
    
    	$(this).bind("plothover", function (event, pos, item) {         
        if (item) {
            if (previousPoint != item.dataIndex) {
                previousPoint = item.dataIndex;

                $("#tooltip").remove();
                
                var x = item.datapoint[0];
                var y = item.datapoint[1];                
                console.log(item);
                showTooltip(item.pageX, item.pageY,
                  Date(item.datapoint[0]).toString()  + "<br/>" + "<strong>" + y + "</strong> (" + item.series.label + ")");
            }
        }
       	 else {
       	     $("#tooltip").remove();
	            previousPoint = null;
	        }
	    });
	};
	
	function showTooltip(x, y, contents) {
	    $('<div id="tooltip">' + contents + '</div>').css({
	        position: 'absolute',
	        display: 'none',
	        top: y + 5,
	        left: x + 20,
	        border: '2px solid #4572A7',
	        padding: '2px',     
	        size: '10',   
	        'background-color': '#fff',
	        opacity: 0.80
	    }).appendTo("body").fadeIn(200);
	}



	// get new API charts

	function getChartData(ft, tt, method, auth, cseq, totag, url, t) {

                var sendData;
                var mydata = {
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
		var result;
		var temp = [];
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
					var set = {};
					var t = item.from_date.split(/[- :]/);
					var date = new Date(t[0], t[1]-1, t[2], t[3], t[4], t[5]);
					var mss = date.getTime() + 7200000; 
					set[mss] = item.total;
					temp.push([mss, item.total]);
				});
                            }
                            else {
                                // alert("Error: " + msg.status);
                            }
                    },  //Error
                    error: function (xhr, str) {
                          // alert("Error");
                    }
                });

		console.log(temp);
		return temp;
		//return result;
           }



       function loadChartData(ftime, ttime) {

            var results = new Array();
	    
            if ($('#chart_authfail').is(':checked')) {            
                 var d3 = getChartData(ftime, ttime,'407', 0, 'INVITE', 0, '<?php echo APILOC;?>statistic/method/all', 0);
                 results.push({data: d3, label: "AuthFail", yaxis: 1,  bars: { show: true } });                 
            }
            
            if ($('#chart_calls').is(':checked')) {            
                 var d1 = getChartData(ftime, ttime, 'INVITE', 0, '', 0, '<?php echo APILOC;?>statistic/method/all', 0);
                 results.push({data: d1, label: "Calls", yaxis: 2, lines: {show: true}, points: { show: true }});
            }
            
            if ($('#chart_packets').is(':checked')) {            
                 var d2 = getChartData(ftime, ttime, 'ALL', 0, '', 0, '<?php echo APILOC;?>statistic/method/all', 0);
                 results.push({data: d2, label: "Packets", lines: { show: true, fill: true }});
            }
            
            if ($('#chart_asr').is(':checked')) {            
                 var asr = getChartData(ftime, ttime, 'asr', 0, '', 0, '<?php echo APILOC;?>statistic/data/all', 1);
                 results.push({data: asr, label: "ASR", yaxis: 1,  lines: { show: true, steps: true }, 
                      color: "rgb(30, 180, 20)", threshold: { below: 60, color: "rgb(200, 20, 30)" }});
            }
            
	    $('#chart1').empty();	   	    
            
            $.plot($("#chart1"), results ,
		{ 
		  xaxes: [ { mode: 'time' } ],
                  yaxes: [ { position: 'left' }, { position: 'right' }],
                  legend: { position: "nw", margin: 10, show: 'true' },
                  grid: { borderWidth: 0, clickable: true, hoverable: true, mouseActiveRadius: 30 }
             });

             $('#chart1').bind("plotclick", function (event, pos, item) {
                 if (item) {
	           if (document.getElementById('from_time')) {
	                 //$("#clickdata").text("You clicked point " + item.dataIndex + " in " + item.series.label + ".");
	                 var a = new Date(item.datapoint[0]-160000);
	                 var b = new Date(item.datapoint[0]+360000);
	                 var from_date = pad(a.getDate())+'-'+pad(a.getMonth()+1)+'-'+a.getFullYear();
	                 var to_date = pad(b.getDate())+'-'+pad(b.getMonth()+1)+'-'+b.getFullYear();
	                 var from_time = pad(a.getUTCHours())+':'+pad(a.getUTCMinutes())+':'+pad(a.getUTCSeconds());
	                 var to_time = pad(b.getUTCHours())+':'+pad(b.getUTCMinutes())+':'+pad(b.getUTCSeconds());
	                 // Set from/to Time based on graph click
	                 document.getElementById('from_time').value = from_time;
	                 document.getElementById('to_time').value = to_time;
	                 // set date
	                 document.getElementById('from_date').value = from_date;
	                 document.getElementById('to_date').value = to_date;
	                 //alert(date+' '+time+' to:'+to_time);
                     }		
                  }
              });
	    $('#chart1').UseTooltip();
              
              var allp = getChartData(ftime,ttime,'ALL','<?php echo APILOC;?>statistic/method/total');
              if(allp.length > 0) $('#chart1').append('<div style="position:absolute;left:35%;top:10px;color:#666;font-size:-2">Captured Frames: '+allp[0][1]+'</div>');
                
          }
          
jQuery(document).ready(function() {          

          loadChartData('<?php echo date("Y-m-d H:i:s", time() - 3600); ?>', '<?php echo date("Y-m-d H:i:s", time());?>');

});



function pad(number) {
     return (number < 10 ? '0' : '') + number;
}  


</script>		

