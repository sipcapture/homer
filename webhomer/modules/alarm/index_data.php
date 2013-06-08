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
<?php //if(!defined('DASHBOARD_VIEW')): 
?>
	<table cellpadding="0" cellspacing="0" border="0" class="display" id="alarmtable">
		<thead>
			<tr>
				<th width="5%">id</th>
				<th width="20%">Date</th>
				<th width="15%">Type</th>
				<th width="15%">IP</th>
				<th width="10%">Total</th>
				<th width="30%">Description</th>
				<th width="5%">Status</th>
				<th width="5%">Delete</th>
			</tr>
		</thead>
		<tbody>		
		</tbody>
		<tfoot>
		</tfoot>
	</table>                    

<?php //else : 
?>
                </div>
 <!--
<div id="chart11" style="min-width: 360px; width: 99%; margin-left: 1px; float: center; height: 220px"></div>
-->

<script type="text/javascript">

// $ = jQuery;

var oTable;
var refreshIntervalId;

var ftime = '<?php echo $from_date; ?>';
var ttime = '<?php echo $to_date; ?>';
var result;

$.fn.dataTableExt.oApi.fnReloadAjax = function ( oSettings, sNewSource, fnCallback, bStandingRedraw )
{
    if ( sNewSource !== undefined && sNewSource !== null ) {
        oSettings.sAjaxSource = sNewSource;
    }
 
    // Server-side processing should just call fnDraw
    if ( oSettings.oFeatures.bServerSide ) {
        this.fnDraw();
        return;
    }
 
    this.oApi._fnProcessingDisplay( oSettings, true );
    var that = this;
    var iStart = oSettings._iDisplayStart;
    var aData = [];
 
    this.oApi._fnServerParams( oSettings, aData );
 
    oSettings.fnServerData.call( oSettings.oInstance, oSettings.sAjaxSource, aData, function(json) {
        /* Clear the old information from the table */
        that.oApi._fnClearTable( oSettings );
 
        /* Got the data - add it to the table */
        var aData =  (oSettings.sAjaxDataProp !== "") ?
            that.oApi._fnGetObjectDataFn( oSettings.sAjaxDataProp )( json ) : json;
 
        for ( var i=0 ; i<aData.length ; i++ )
        {
            that.oApi._fnAddData( oSettings, aData[i] );
        }
         
        oSettings.aiDisplay = oSettings.aiDisplayMaster.slice();
 
        that.fnDraw();
 
        if ( bStandingRedraw === true )
        {
            oSettings._iDisplayStart = iStart;
            that.oApi._fnCalculateEnd( oSettings );
            that.fnDraw( false );
        }
 
        that.oApi._fnProcessingDisplay( oSettings, false );
 
        /* Callback user function - for event handlers etc */
        if ( typeof fnCallback == 'function' && fnCallback !== null )
        {
            fnCallback( oSettings );
        }
    }, oSettings );
};

function updateAlarm(status,id) {

                var sendData;
                var mydata = {
                   status: status,
                   id: id
                };
                
                var url = '<?php echo APILOC."alarm/update"?>';
                

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
                                oTable.fnReloadAjax();                                                                                                
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
                //return true;
}

function deleteAlarm(id) {

                var sendData;
                var mydata = {
                   id: id
                };
                
                var url = '<?php echo APILOC."alarm/delete"?>';
                
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
                                oTable.fnReloadAjax();                                                                                                
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
              //return true;
}


function loadAlarmData(ft, tt, status, interval) {

        var sendData;
	var url = '<?php echo APILOC."alarm/data/all"?>';
        
        var mydata = {
                   status: status,
                   from_datetime: ft,
                   to_datetime: tt
	};

	if(mydata != null) sendData = '?data=' + $.toJSON(mydata);        

	clearInterval(refreshIntervalId);

	oTable.fnReloadAjax(url+sendData);
    
        if(interval > 0) {
        
            alert('Autorefresh active');
            refreshIntervalId =  setInterval(function() {
                    oTable.fnReloadAjax(url+sendData);            
            },(interval*1000));                
            
        }                        
}

jQuery(document).ready(function($) {

	// get new API charts
	oTable = $('#alarmtable').dataTable( {
		"bProcessing": true,
		"bLengthChange": false,
		"bAutoWidth": true,
		"aaSorting": [[ 0, "desc" ]],
	        "sAjaxSource": '<?php echo APILOC."alarm/data/all"?>',
	        "sAjaxDataProp": "data",
	        "iDisplayLength": 50,
 	        "aoColumns": [
                        { "mData": "id"},
	                { 
	                    "mData": "create_date",
                            "mRender": function ( data, type, full ) {
        	                     // return "<center>"+data+"</center>";
                               // render date + link (wip)
                               var time = data.split(/\s+/g);
                               var nt = time[1].split(':');
                               var diff = 1; // minutes
                               var diff2 = 0;
                               var source_ip = "";
                               var method = "";

                               // from 
                               if ( nt[1] >= 1 && nt[1] < 59 ) {
                                      var ntime = nt[0]+":"+(parseInt(nt[1])-diff)+":"+nt[2];
                               } else {
                                      var ntime = (parseInt(nt[0])-diff)+":59:"+nt[2];
                               }
                               // to 
                               if ( nt[1] >= 1 && nt[1] < 58 ) {
                                      var ttime = nt[0]+":"+(parseInt(nt[1])+diff2)+":"+nt[2];
                               } else {
                                      var ttime = (parseInt(nt[0])+diff2)+":59:"+nt[2];
                               }
                               //console.log(time[1]+" "+ttime);
                               if(full.source_ip != "0.0.0.0") source_ip="source_ip="+full.source_ip+"&";
                               if(full.type.indexOf("Too Many ") !== -1) method="method="+full.type.substring(9)+"&";

                            return "<center><a href='?"+method+source_ip+"location[]=1&node=&from_date="+time[0]+"&from_time="+ntime+"&to_date="+time[0]+"&to_time="+ttime+"&limit=100&task=result&component=search'>"+data+"</a></center>";   
        	             }
	                },
	                { 
                            "mData": "type",
                            "mRender": function ( data, type, full ) {
                                    return "<center>"+data+"</center>";
                            }
	                },
	                { 
	                    "mData": "source_ip",
                            "mRender": function ( data, type, full ) {
        	                     return "<center>"+data+"</center>";   
        	             }
	                },
	                { 
	                    "mData": "total",
        	            "mRender": function ( data, type, full ) {
        	                     return "<center>"+data+"</center>";   
        	             }
	                },
	                { "mData": "description" },
	                { "mData": "status",
                          "mRender": function ( data, type, full ) {
                                  if(data == 1) {   
                                      return '<a href="javascript:updateAlarm(0,'+full.id+');"><font color="red">New</font></a>';
                                  }
                                  else {
                                      return '<a href="javascript:updateAlarm(1,'+full.id+');">Old</a>';
                                  }
                           }
	                },
	                { "mData": "status",
                          "mRender": function ( data, type, full ) {
                                  return '<a href="javascript:deleteAlarm('+full.id+');"><font color="black">Delete</font></a>';
                          }
                        }
	        ]
        } );        
});


</script>		

