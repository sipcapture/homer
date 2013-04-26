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

<?php if(!defined('DASHBOARD_VIEW')): ?>            
		<div id="chart8" style="min-width: 90px; width: 45%; margin-left: 1px; float: left; height: 220px;"></div>
		<div id="chart9" style="min-width: 90px; width: 45%; margin-left: 1px; float: left; height: 220px;"></div>
<?php else : ?>
		<div id="chart8" style="min-width: 90px; width: 100%; margin-left: 1px; float: left; height: 220px;"></div>
<?php endif ?>    
		</div>
 
<div id="chart11" style="min-width: 360px; width: 99%; margin-left: 1px; float: center; height: 220px"></div>

<script type="text/javascript">

// $ = jQuery;



	// get new API charts

        var ftime = '<?php echo $from_date; ?>';
        var ttime = '<?php echo $to_date; ?>';
        var result;

        function getUASData(ft,tt,method,url,cseq) {

                var sendData;
                var mydata = {
                   method: method,
                   from_datetime: ft,
                   to_datetime: tt
                };

                if(mydata != null) sendData = 'data=' + $.toJSON(mydata);
                var temp = [];
                var uas = new Array();

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
                                    uas.push({label: item.useragent, data: parseInt(item.total)});
                                });
                            }
                            else {
				console.log('ERROR:' +msg.status);
				uas.push({label: "NO DATA", data: 100});
                                // alert("Error: " + msg.status);
                            }
                    },  //Error
                    error: function (xhr, str) {
                          // alert("Error");
                    }
                });

                // return temp;
                return uas;
           }


    function loadUASData(ftime, ttime) {


	var uas_reg = getUASData(ftime,ttime,'REGISTER','<?php echo APILOC;?>statistic/useragent/total');

	$.plot($("#chart8"), uas_reg,
	{
        	series: {
	            pie: {
			radius: 0.8,
        	        show: true,
			combine: {
                    		color: '#999',
                    		threshold: 0.005
	                }
	            }
        	},
                grid: {
                        hoverable: true,
                        clickable: true
                },
	        legend: {
        	    show: true,
	            labelFormatter: function(label, series) {
                	 return ' ' + label.slice(0,30) + ' ('+Math.round(series.percent)+'%)';
	                }
        	}
	});
	
	$('#chart8').append('<div style="position:absolute;left:5px;top:215px;color:#000;font-size:-2">User Agent by REGISTER</div>');
	
<?php if(!defined('DASHBOARD_VIEW')) : ?>

	var uas_inv = getUASData(ftime,ttime,'INVITE','<?php echo APILOC;?>statistic/useragent/total');
	$.plot($("#chart9"), uas_inv,
	{
        	series: {
	            pie: {
			radius: 0.8,
        	        show: true,
                        combine: {
                                color: '#999',
                                threshold: 0.005
                        }
	            }
        	},
                grid: {
                        hoverable: true,
                        clickable: true
                },
	        legend: {
        	    show: true,
	            labelFormatter: function(label, series) {
                	 return ' ' + label.slice(0,30) + ' ('+Math.round(series.percent)+'%)';
	                }
        	}
	});	
	
	 $('#chart9').append('<div style="position:absolute;left:5px;top:215px;color:#000;font-size:-2">User Agent by INVITE</div>');
	
<?php endif ?>
}


$(document).ready(function() {
    loadUASData('<?php echo date("Y-m-d H:i:s", time() - 3600); ?>', '<?php echo date("Y-m-d H:i:s", time());?>');

});


</script>		

