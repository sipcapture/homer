/*
 * HOMER Web Interface
 * Homer's JS Core
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

<!--

function saveCancel() {

        history.go(-1);
        return true;
}

function showSearch() {

    
    if(jQuery('#searchTFoot').is(':visible')) jQuery('#searchTFoot').hide('slow');
    else jQuery('#searchTFoot').show('slow');

}



function setMethod(qmethod) {

	meth = document.getElementById('method');
        meth.value = qmethod;
}


function check_form() {

	regexpr = /(\d{2})-(\d{2})-(\d{4})/g;
	dateFromArray = regexpr.exec(document.getElementById('from_date').value); 
	regexpr = /(\d{2})-(\d{2})-(\d{4})/g;
	dateToArray = regexpr.exec(document.getElementById('to_date').value); 

	from_date = dateFromArray[2] + '/' +dateFromArray[1] + '/' +dateFromArray[3];
	to_date = dateToArray[2] + '/' +dateToArray[1] + '/' +dateToArray[3];
	
        from_time = document.getElementById('from_time').value;
        to_time = document.getElementById('to_time').value;
        var dtStart = new Date(from_date + " " + from_time);
        var dtEnd = new Date(to_date + " " + to_time);

        if ((dtEnd - dtStart) < 0) {
                document.getElementById('to_time').focus();
                alert("End time is before start time!");
                return false;
        }
        
        var g = document.getElementsByName('location[]');
        var ok = 0;
        for(var i = 0; i < g.length; i++)
	{
                if(g[i].checked) ok=1;
        }
        
        if(!ok) {
            alert("Select minimum one node!");
            return false;        
        }
        
	document.getElementById('task').value="result";
        document.homer.submit();
        return true;
}


function showStats(type) {

	regexpr = /(\d{2})-(\d{2})-(\d{4})/g;
	dateFromArray = regexpr.exec(document.getElementById('from_date').value); 
	regexpr = /(\d{2})-(\d{2})-(\d{4})/g;
	dateToArray = regexpr.exec(document.getElementById('to_date').value); 

	from_date = dateFromArray[2] + '/' +dateFromArray[1] + '/' +dateFromArray[3];
	to_date = dateToArray[2] + '/' +dateToArray[1] + '/' +dateToArray[3];
	
        from_time = document.getElementById('from_time').value;
        to_time = document.getElementById('to_time').value;
        var dtStart = new Date(from_date + " " + from_time);
        var dtEnd = new Date(to_date + " " + to_time);

        if ((dtEnd - dtStart) < 0) {
                document.getElementById('to_time').focus();
                alert("End time is before start time!");
                return false;
        }
        
        from_date = dateFromArray[3] + '-' +dateFromArray[2] + '-' +dateFromArray[1];
        to_date = dateToArray[3] + '-' +dateToArray[2] + '-' +dateToArray[1];                 

	document.getElementById('task').value="result";

	if(type == "all" || type == "dataCharts") {
            loadChartData(from_date + " " + from_time, to_date + " " + to_time);
        }
        
        if(type == "all" || type == "dataQoS") {
            loadQoSData(from_date + " " + from_time, to_date + " " + to_time);
        }            
        
        if(type == "all" || type == "dataUAS") {
            loadUASData(from_date + " " + from_time, to_date + " " + to_time);
        }            
        
        if(type == "all" || type == "dataIP") {
            loadIPData(from_date + " " + from_time, to_date + " " + to_time);
        }            
        
        if(type == "all" || type == "dataAlarm") {
            //loadAlarmData(from_date + " " + from_time, to_date + " " + to_time);
            loadAlarmData();
        }            
            
        return false;
}

function makeSearch(type) {

	regexpr = /(\d{2})-(\d{2})-(\d{4})/g;
	dateFromArray = regexpr.exec(document.getElementById('from_date').value); 
	regexpr = /(\d{2})-(\d{2})-(\d{4})/g;
	dateToArray = regexpr.exec(document.getElementById('to_date').value); 

	from_date = dateFromArray[2] + '/' +dateFromArray[1] + '/' +dateFromArray[3];
	to_date = dateToArray[2] + '/' +dateToArray[1] + '/' +dateToArray[3];
	
        from_time = document.getElementById('from_time').value;
        to_time = document.getElementById('to_time').value;
        var dtStart = new Date(from_date + " " + from_time);
        var dtEnd = new Date(to_date + " " + to_time);

        if ((dtEnd - dtStart) < 0) {
                document.getElementById('to_time').focus();
                alert("End time is before start time!");
                return false;
        }
        
        from_date = dateFromArray[3] + '-' +dateFromArray[2] + '-' +dateFromArray[1];
        to_date = dateToArray[3] + '-' +dateToArray[2] + '-' +dateToArray[1];                 

	var g = document.getElementsByName('location[]');
        var ok = 0;
        for(var i = 0; i < g.length; i++)
        {
                if(g[i].checked) ok=1;
        }

        if(!ok) {
            alert("Select minimum one node!");
            return false;
        }
         
        document.getElementById('task').value="result";
        document.getElementById('component').value="search";
        document.homer.submit();
           
        return false;
}

function showAlarms(type) {

	regexpr = /(\d{2})-(\d{2})-(\d{4})/g;
	dateFromArray = regexpr.exec(document.getElementById('from_date').value); 
	regexpr = /(\d{2})-(\d{2})-(\d{4})/g;
	dateToArray = regexpr.exec(document.getElementById('to_date').value); 

	from_date = dateFromArray[2] + '/' +dateFromArray[1] + '/' +dateFromArray[3];
	to_date = dateToArray[2] + '/' +dateToArray[1] + '/' +dateToArray[3];
	
        from_time = document.getElementById('from_time').value;
        to_time = document.getElementById('to_time').value;
        var dtStart = new Date(from_date + " " + from_time);
        var dtEnd = new Date(to_date + " " + to_time);

        if ((dtEnd - dtStart) < 0) {
                document.getElementById('to_time').focus();
                alert("End time is before start time!");
                return false;
        }
        
        from_date = dateFromArray[3] + '-' +dateFromArray[2] + '-' +dateFromArray[1];
        to_date = dateToArray[3] + '-' +dateToArray[2] + '-' +dateToArray[1];                 

	document.getElementById('task').value="result";
		
	var status = parseInt($('#alarmtype').val());	
	var refresh = parseInt($('#refresh').val());
	
	if(status == 2) status = undefined;	
        loadAlarmData(from_date + " " + from_time, to_date + " " + to_time, status, refresh);
            
        return false;
}





function checkboxEvent(ms, id) {

	var isVisible = $('#deltacalc').is(':visible');
	if(isVisible) {
		calculateDelta(ms);
	}
	//else {		
	//	alert("Just checked:["+id+"]");
	//}
}

function toggleDelta() {

	var g = document.getElementsByName('cid[]');
	for(var i = 0; i < g.length; i++)
        {
                        if(g[i].checked) g[i].checked=false;
        }

	$('#deltacalc').toggle('slow');	
}


function calculateDelta(ms) {

	var a = document.getElementById('delta_value_1');
	var b = document.getElementById('delta_value_2');
	var c = document.getElementById('delta_result');
	if(a.value == "") a.value = ms;
	else if(b.value == "") { 
		b.value = ms;
		c.value = b.value*1 - a.value*1;

	}
	else {
		
		var g = document.getElementsByName('cid[]');
        	for(var i = 0; i < g.length; i++)
	        {
        	        if(g[i].checked) g[i].checked=false;
                }

		a.value = "";
		b.value = "";
		c.value = "";
	} 
}

function sipSendForm() {

	var phpsip_to = $('#phpsip_to').val();var phpsip_from = $('#phpsip_from').val();var phpsip_prox = $('#phpsip_prox').val();
	var phpsip_meth = $('#phpsip_meth').val();var phpsip_head = $('#phpsip_head').val();

	adminAction('sipsend', 'to='+phpsip_to+'&from='+phpsip_from+'&proxy='+phpsip_prox+'&method='+phpsip_meth+'&head='+phpsip_head);
}

function sipKillVic() {

	var sipvic_ip = $('#vicious_ip').val();var sipvic_port = $('#vicious_port').val();
	//console.log('svcrash.py -d '+sipvic_ip+' -p '+sipvic_port);
	adminAction('sipVic', 'dest='+sipvic_ip+'&port='+sipvic_port);
}

function adminAction(task, action, title) {

   var url = "utils.php?task="+task+"&"+action;
   if (!title) {var title = 'Result:'}
			var popup = $('<div id="popup"></div>')
                        .load(url, '', function(response, status, xhr) {
                        if (status == 'error') {
                        var msg = "Sorry but there was an error: ";
                        $(".content").html(msg + xhr.status + " " + xhr.statusText);
                        }})
			.dialog({
                                autoOpen: true,
				stack: false,
				width: 'auto',
				position: [10, 80],
				height: 'auto',
				open: function(e, i) {  $(this).parents(".ui-dialog:first").find(".ui-dialog-titlebar").addClass("ui-state-highlight"); },	
				close: function(e, i) { $(this).remove(); },	
                                title: title
                        })
			.css('zIndex', -1)
			.focus();			

}

function popMessage(id) {

			var url = "utils.php?task=sipmessage&id="+id;
			var posx = $('body').data('posx');
			var posy = $('body').data('posy');

			jQuery('<div id="'+id+'"></div>').appendTo( jQuery('body') );
			$("#"+id)
                        //var pflow = $('<div id="'+id+'"></div>')
			.load(url, '', function(response, status, xhr) {
                        if (status == 'error') {
                        var msg = "Sorry but there was an error: ";
                        $(".content").html(msg + xhr.status + " " + xhr.statusText);
                        }})
                        .dialog({
                                autoOpen: true,
				autoResize: true, 
				stack: true,
                                width: 500,
                                // height: 300,
				minHeight: 350, 
                                height: 'auto',
				position: [posx + 40, posy -5],
				open: function(e, i) { 
					$(this).css({ overflow: 'hidden' }); 
					$(this).css({ height: 'auto' }); 
				},
				open: function(e, i) {  $(this).parents(".ui-dialog:first").find(".ui-dialog-titlebar").addClass("ui-state-highlight"); },	
                                close: function(e, i) { $(this).remove(); },
                                title: 'MSG ID: '+id
                        })
			.css('zIndex', -1)
			.focus();

						
			document.getElementById(id).focus(); 
}

function popAny(url,title) {

			//var url = "utils.php?task=sipmessage&id="+id;
			if (!title) {var title = ''; }
			var posx = $('body').data('posx');
			var posy = $('body').data('posy');
			var id = (new Date()).getTime();
			jQuery('<div id="'+id+'"></div>').appendTo( jQuery('body') );
			$("#"+id)
                        //var pflow = $('<div id="'+id+'"></div>')
			.load(url, '', function(response, status, xhr) {
                        if (status == 'error') {
                        var msg = "Sorry but there was an error: ";
                        $(".content").html(msg + xhr.status + " " + xhr.statusText);
                        }})
                        .dialog({
                                autoOpen: true,
				stack: true,
                                width: 500,
                                height: 'auto',
				position: [posx + 40, posy -5],
				open: function(e, i) { 
					$(this).css({ overflow: 'hidden' }); 
				},
				open: function(e, i) {  $(this).parents(".ui-dialog:first").find(".ui-dialog-titlebar").addClass("ui-state-highlight"); },	
                                close: function(e, i) { $(this).remove(); },
                                title: title
                        })
			.css('zIndex', -1)
			.focus();

						
			document.getElementById(id).focus(); 
}


function showCallFlow(id,table,tnode,location,unique,tag,callid,fdate,tdate, ft, tt, td, b2b) {

  if ( callid.match(/-0$/) )  { callid = callid.replace(/-0$/,""); }

	  var url = "cflow.php?cid[]="+callid;

	  if (fdate != undefined) {
		  url += "&from_time="+ft+"&to_time="+tt;
		  url += "&from_date="+fdate+"&to_date="+tdate;
	  }
	  url += "&callid_aleg="+b2b;
			var posx = $('body').data('posx');
                        var posy = $('body').data('posy');

			var cflow = $('<div id="cflow"></div>')
                        .load(url, '', function(response, status, xhr) {
                        if (status == 'error') {
                        var msg = "Sorry but there was an error: ";
                        $(".content").html(msg + xhr.status + " " + xhr.statusText);
                        }})
			.dialog({
                                autoOpen: true,
                                // autoResize: true,
				stack: false,
				width: 'auto',
				position: [posx-300, posy-80],
				// height: 'auto',
				height: 500,
				minHeight: 300,
				close: function(e, i) { $(this).remove(); },	
				open: function(e, i) {  
					$(this).parents(".ui-dialog:first").find(".ui-dialog-titlebar").addClass("ui-state-highlight"); 
				//	$(this).css({ height: 'auto' });
				},	
                                title: 'Call Flow: '+callid
                        })
			.css('zIndex', -1)
			.focus();			
}

function showCallFlow2(type, callid, url) {


		var posx = $('body').data('posx');
                var posy = $('body').data('posy');	  
                
                var g = document.getElementsByName('cid[]');
		var found = 0;

	        for(var i = 0; i < g.length; i++)
        	{
	            if(g[i].checked) {
                 var cellText = $('#'+g[i].id).parent().parent().find("a[alt=\"callflow\"]").text();		
                 /* if(found == 0 && cellText != callid) {  url += "&cid[]="+cellText; found = 1; } */                 
                  /* g[i].checked=false;*/
                 /* Add all unique cids selected when showing multiple call flows. Thanks Gareth Aeriandi */
                 if(url.indexOf(cellText) === -1) {
                       url += "&cid[]="+cellText;
                 }
	            }
        	}

		if(type == 1) {
			var cflow = jQuery('<div id="cflow"></div>').load(url, '', function(response, status, xhr) {
                           if (status == 'error') {
                                        var msg = "Sorry but there was an error: ";
                                        $(".content").html(msg + xhr.status + " " + xhr.statusText);
                                }}).dialog({
                                        autoOpen: true,
                                        // autoResize: true,
                                        stack: true,
                                        width: 'auto',
                                        position: [posx-300, posy-80],
                                        // height: 'auto',
                                        height: 500,
                                        minHeight: 300,
                                        close: function(e, i) { $(this).remove(); },
                                        open: function(e, i) {
						$(this).css({ overflow: 'hidden' });
	                                        //$(this).css({ height: 'auto' });
	                                        //$(this).parents(".ui-dialog:first").find(".ui-dialog-titlebar").addClass("ui-state-highlight");
                                       		 //$(this).css({ height: 'auto' });
                                        },
                                        title: 'Call Flow: '+callid
                                }).css('zIndex', -1).focus();
		}
		else {
		
			settings = { height:600, width:600, toolbar:0, scrollbars:1, status:0, 
				resizable:1, left:0, top:0, center:1, location:0, menubar:0
			};

			if (settings.center == 1){
				settings.top = (screen.height-(settings.height + 110))/2;
				settings.left = (screen.width-settings.width)/2;
			}
		
			parameters = "location=" + settings.location + ",menubar=" + settings.menubar + ",height=" + settings.height + ",width=" + settings.width + ",toolbar=" + settings.toolbar + ",scrollbars=" + settings.scrollbars  + ",status=" + settings.status + ",resizable=" + settings.resizable + ",left=" + settings.left  + ",screenX=" + settings.left + ",top=" + settings.top  + ",screenY=" + settings.top;
			var winObj = window.open(url, 'x'+(Math.random() * 10000).toFixed(0), parameters);			
			winObj.focus();			
		}                		
}

function popMessage2(type, id, url) {
			
			var posx = $('body').data('posx');
			var posy = $('body').data('posy');
			
			if(type == 1) {
				jQuery('<div id="'+id+'"></div>').appendTo( jQuery('body') );
				$("#"+id)
                	        //var pflow = $('<div id="'+id+'"></div>')
				.load(url, '', function(response, status, xhr) {
	                        if (status == 'error') {
        	                var msg = "Sorry but there was an error: ";
                	        $(".content").html(msg + xhr.status + " " + xhr.statusText);
	                        }})
        	                .dialog({
                	                autoOpen: true,
					autoResize: true, 
					stack: true,
	                                width: 500,
        	                        // height: 300,
					minHeight: 350, 
                        	        height: 'auto',
					position: [posx + 40, posy -5],
					open: function(e, i) { 
						$(this).css({ overflow: 'hidden' }); 
						$(this).css({ height: 'auto' }); 
					},
					open: function(e, i) {  $(this).parents(".ui-dialog:first").find(".ui-dialog-titlebar").addClass("ui-state-highlight"); },	
        	                        close: function(e, i) { $(this).remove(); },
                	                title: 'MSG ID: '+id
                        	}).css('zIndex', -1).focus();						
				document.getElementById(id).focus(); 
			}
			else {
		
				settings = { height:300, width:500, toolbar:0, scrollbars:1, status:0, 
					resizable:1, left:[posy -5], top:[posx + 40], center:0, location:0, menubar:0
				};

				if (settings.center == 1){
					settings.top = (screen.height-(settings.height + 110))/2;
					settings.left = (screen.width-settings.width)/2;
				}
			
				parameters = "location=" + settings.location + ",menubar=" + settings.menubar + ",height=" + settings.height + ",width=" + settings.width + ",toolbar=" + settings.toolbar + ",scrollbars=" + settings.scrollbars  + ",status=" + settings.status + ",resizable=" + settings.resizable + ",left=" + settings.left  + ",screenX=" + settings.left + ",top=" + settings.top  + ",screenY=" + settings.top;		
				var name = 'Call ID: '+id;
				winObj = window.open(url, name, parameters);			
				winObj.focus();			
			}
}


function checkAnswer(id, value) {

          var submit = 0;
          var search=/ (search|suchen|go|submit)/gi;
          if(value.match(search)){
               submit =1;
               value = value.replace(search, "");
          }

          value = value.replace(/ /,"");
          document.getElementById(id).value = value;           
          if(submit == 1) document.homer.submit();
}


function clear_complete_form() {


	// User
        document.getElementById('ruri_user').value="";
        document.getElementById('to_user').value="";
        document.getElementById('from_user').value="";
        document.getElementById('pid_user').value="";
        document.getElementById('contact_user').value="";
        document.getElementById('auth_user').value="";
        document.getElementsByName('logic_or')[0].checked=false;
	
	//Call
        document.getElementById('callid').value="";
        document.getElementById('callid_aleg')[0].checked=false;
        document.getElementById('from_tag').value="";
        document.getElementById('to_tag').value="";
        document.getElementById('via_1_branch').value="";
        document.getElementById('method').value="";
        document.getElementById('reply_reason').value="";

	//Header
        document.getElementById('ruri').value="";
        document.getElementById('via_1').value="";
        document.getElementById('diversion').value="";
        document.getElementById('cseq').value="";
        document.getElementById('reason').value="";
        document.getElementById('content-type').value="";
        document.getElementById('authorization').value="";
        document.getElementById('user_agent').value="";
        document.getElementById('msg').value="";

	//Time
        document.getElementsByName('location')[0].selected=true;
        document.getElementsByName('date')[0].selected=true;
        document.getElementById('max_records').value="100";

	//Network
        document.getElementById('source_ip').value="";
        document.getElementById('source_port').value="";
        document.getElementById('destination_ip').value="";
        document.getElementById('destination_port').value="";
        document.getElementById('contact_ip').value="";
        document.getElementById('contact_port').value="";
        document.getElementById('originator_ip').value="";
        document.getElementById('originator_port').value="";
        document.getElementsByName('proto')[0].selected=true;
        return true;
}


function clear_form() {


	// User
        document.getElementById('ruri_user').value="";
        document.getElementById('to_user').value="";
        document.getElementById('from_user').value="";
        document.getElementById('pid_user').value="";
        document.getElementsByName('logic_or')[0].checked=false;
	
	//Call
        document.getElementById('callid').value="";

	//Time
        document.getElementsByName('location')[0].selected=true;
        document.getElementsByName('date')[0].selected=true;
        document.getElementById('max_records').value="100";

        return true;
}



function MM_openBrWindow(theURL,winName,features) { //v2.0
          window.open(theURL,winName,features);
}


// Extended Tooltip Javascript
// copyright 9th August 2002, 3rd July 2005, 24th August 2008
// by Stephen Chapman, Felgall Pty Ltd

// permission is granted to use this javascript provided that the below code is not altered
function pw() {return window.innerWidth || document.documentElement.clientWidth || document.body.clientWidth}; function mouseX(evt) {return evt.clientX ? evt.clientX + (document.documentElement.scrollLeft || document.body.scrollLeft) : evt.pageX;} function mouseY(evt) {return evt.clientY ? evt.clientY + (document.documentElement.scrollTop || document.body.scrollTop) : evt.pageY} function popUp(evt,oi) {if (document.getElementById) {var wp = pw(); dm = document.getElementById(oi); ds = dm.style; st = ds.visibility; if (dm.offsetWidth) ew = dm.offsetWidth; else if (dm.clip.width) ew = dm.clip.width; if (st == "visible" || st == "show") { ds.visibility = "hidden"; } else {tv = mouseY(evt) + 20; lv = mouseX(evt) - (ew/4); if (lv < 2) lv = 2; else if (lv + ew > wp) lv -= ew/2; lv += 'px';tv += 'px';  ds.left = lv; ds.top = tv; ds.visibility = "visible";}}}
                  

function saveCancel() {

	history.go(-1);
        return true;
}


// JS Calendar
var calendar = null; // remember the calendar object so that we reuse
// it and avoid creating another

// This function gets called when an end-user clicks on some date
function selected(cal, date) {
        cal.sel.value = date; // just update the value of the input field
}

// And this gets called when the end-user clicks on the _selected_ date,
// or clicks the "Close" (X) button.  It just hides the calendar without
// destroying it.
function closeHandler(cal) {
        cal.hide();                     // hide the calendar

        // don't check mousedown on document anymore (used to be able to hide the
        // calendar when someone clicks outside it, see the showCalendar function).
        Calendar.removeEvent(document, "mousedown", checkCalendar);
}

// This gets called when the user presses a mouse button anywhere in the
// document, if the calendar is shown.  If the click was outside the open
// calendar this function closes it.
function checkCalendar(ev) {
        var el = Calendar.is_ie ? Calendar.getElement(ev) : Calendar.getTargetElement(ev);
        for (; el != null; el = el.parentNode)
        // FIXME: allow end-user to click some link without closing the
        // calendar.  Good to see real-time stylesheet change :)
        if (el == calendar.element || el.tagName == "A") break;
        if (el == null) {
                // calls closeHandler which should hide the calendar.
                calendar.callCloseHandler(); Calendar.stopEvent(ev);
        }
}

// This function shows the calendar under the element having the given id.
// It takes care of catching "mousedown" signals on document and hiding the
// calendar if the click was outside.
function showCalendar(id) {
        var el = document.getElementById(id);
        if (calendar != null) {
                // we already have one created, so just update it.
                calendar.hide();                // hide the existing calendar
                calendar.parseDate(el.value); // set it to a new date
        } else {
                // first-time call, create the calendar
                var cal = new Calendar(true, null, selected, closeHandler);
                calendar = cal;         // remember the calendar in the global
                cal.setRange(1900, 2070);       // min/max year allowed
                calendar.create();              // create a popup calendar
        }
        calendar.sel = el;              // inform it about the input field in use

	var x = el.offsetLeft
	var y =  el.offsetTop;
	calendar.setDateFormat('dd.mm.y');

        calendar.showAt(x + 400, y + 500);

        // catch mousedown on the document
        Calendar.addEvent(document, "mousedown", checkCalendar);
        return false;
}

function mktime() {
    // *     example 1: mktime(14, 10, 2, 2, 1, 2008);
    // *     returns 1: 1201871402
    // *     example 2: mktime(0, 0, 0, 0, 1, 2008);
    // *     returns 2: 1196463600
    
    var no, ma = 0, mb = 0, i = 0, d = new Date(), argv = arguments, argc = argv.length;
    d.setHours(0,0,0); d.setDate(1); d.setMonth(1); d.setYear(1972);
 
    var dateManip = {
        0: function(tt){ return d.setHours(tt); },
        1: function(tt){ return d.setMinutes(tt); },
        2: function(tt){ set = d.setSeconds(tt); mb = d.getDate() - 1; return set; },
        3: function(tt){ set = d.setMonth(parseInt(tt)-1); ma = d.getFullYear() - 1972; return set; },
        4: function(tt){ return d.setDate(tt+mb); },
        5: function(tt){ return d.setYear(tt+ma); }
    };
    
    for( i = 0; i < argc; i++ ){
        no = parseInt(argv[i]*1);
        if (isNaN(no)) {
            return false;
        } else {
            // arg is number, let's manipulate date object
            if(!dateManip[i](no)){
                // failed
                return false;
            }
        }
    }
 
    return Math.floor(d.getTime()/1000);
}

function showRtcpStats(corr_id, from_time, to_time, apiurl, winid, codec) {

	var sendData;
             var mydata = {
                from_datetime: from_time,
                to_datetime: to_time,
                correlation_id: corr_id
	};
                
	var url = apiurl + "rtcp/report/all";
		  
	var jitter = new Array();
	var packetlost = new Array();
	var mosarray = new Array();
	
	if(mydata != null) sendData = 'data=' + $.toJSON(mydata);

        // Send
        $.ajax({ 
                    url: url,
                    type: 'GET',
                    async: false,
                    dataType: 'json',
                    data: sendData,
                    success: function (msg,code) {
                                //Success
                            $('#rtcpinfodata'+winid).empty();      
                                             
                            var table = "";
                            table +='Report:<BR><table border="1" id="data" cellspacing="0" width="95%" style="background: #f9f9f9;">';     
                            table += "<tr>";
                            table += "<th>ID&nbsp;</th>";
                            table += "<th>Type&nbsp;</th>";
                            table += "<th>TS&nbsp;</th>";
                            table += "<th>Jitter&nbsp;</th>";
                            table += "<th>Packetloss&nbsp;</th>";
                            table += "<th>Fraction PL&nbsp;</th>";
                            table += "<th>Packets&nbsp;</th>";
                            table += "<th>MOS</th>";
                            table += "</tr>";
                            if(msg.status == 'ok') {    
                                 var data = msg.data;
                                 $('#callflow'+winid).hide(400);
                                 $('#rtpinfo'+winid).hide(400);                           
                                 $('#rtcpinfo'+winid).show(400);                                                            
                                 //console.log(msg.data); 
                                 $.each( data, function( index, val ) {
                                   //table += val.id+':<BR>');
                                   //table += val.msg+'<BR>');
                                   
                                   var rtcpobj = jQuery.parseJSON( val.msg );
                                   var ts;
                                   var totalpkts;
				   var si = rtcpobj.sender_information;
				   if(si) {
				   	ts = parseInt((si.ntp_timestamp_sec * 1000000 + si.ntp_timestamp_usec)/1000);
				   	totalpkts = si.packets;
				   }
				   else {
				   	ts = parseInt((msg.micro_ts)/1000);
				   	totalpkts = 0;
				   }
				   var rp = rtcpobj.report_blocks;
				   
				   var msgts = parseInt(msg.data[index].micro_ts/1000);
				   
				   //console.log(rp[0]);
				   
				   if(rp[0]) {
				       var pl = rp[0].packets_lost;
				       var frpl = rp[0].fraction_lost;
				       var jt = rp[0].ia_jitter;
				       var type = rtcpobj.type;
				       //FAKE!!! We need to have real RTT. here is RTCP round trip
				       var rtt = parseInt(rp[0].dlsr/65536);

				       packetlost.push([msgts, frpl]);
				       jitter.push([msgts, jt]);				       				       
				
				       //var mos = calculateMOS(jt, pl, rtt, 120)
				       var mos = calculatePlMos(frpl, codec);
                                       mosarray.push([msgts, mos]);				       				       				       
                                       
                                       color = "green";
                                       if(mos < 3) color="red";
                                                                                
                                       table += "<tr>";
                                       table += "<td align='center'>"+val.id+"</td>";
                                       table += "<td align='center'>"+type+"</td>";
                                       table += "<td align='center'>"+ts+"</td>";
                                       table += "<td align='center'>"+jt+"</td>";
                                       table += "<td align='center'>"+pl+"</td>";
                                       table += "<td align='center'>"+frpl+"</td>";
                                       table += "<td align='center'>"+totalpkts+"</td>";
                                       table += "<td align='center'><font color='"+color+"'>"+mos+"</font></td>";
                                       table += "</tr>";
                                   }
                                                                      
                                 });                                                                  
                                 
                                 table += "</table>";
                                 $('#rtcpinfodata'+winid).html(table);      

                                 makeRtcpChart(winid, jitter, packetlost, mosarray);
                                 
                            }
                            else {
                                //console.log('ERROR:' +msg.status);
                                alert("No data");
                            }
                    },  //Error
                    error: function (xhr, str) {
                          // alert("Error");
                    }
        });
}

function showCdrInfo(corr_id, from_time, to_time, apiurl, winid, codec) {

	var sendData;
             var mydata = {
                from_datetime: from_time,
                to_datetime: to_time,
                correlation_id: corr_id
	};
                
	var url = apiurl + "cdr/report/all";
		  
	var jitter = new Array();
	var packetlost = new Array();
	var mosarray = new Array();
	
	if(mydata != null) sendData = 'data=' + $.toJSON(mydata);

        // Send
        $.ajax({ 
                    url: url,
                    type: 'GET',
                    async: false,
                    dataType: 'json',
                    data: sendData,
                    success: function (msg,code) {
                                //Success
                            $('#cdrinfodata'+winid).empty();                       
                            var table = "";
                            
                            table +='Report:<BR><table border="1" id="data" cellspacing="0" width="95%" style="background: #f9f9f9;">';     
                            table +="<tr>";
                            table +="<th>ID&nbsp;</th>";
                            table +="<th>Type&nbsp;</th>";
                            table +="<th>Value&nbsp;</th>";
                            table +="</tr>";
                            if(msg.status == 'ok') {    
                                 var data = msg.data;
                                 $('#callflow'+winid).hide(400);
                                 $('#rtpinfo'+winid).hide(400);                           
                                 $('#rtcpinfo'+winid).hide(400);                                                            
                                 $('#cdrinfo'+winid).show(400);                                                            
                                 //console.log(msg.data); 
                                 $.each( data, function( index, val ) {

                                   var rtcpobj = jQuery.parseJSON( val.msg );
                                   
				   var i = 0;
                                   $.each( rtcpobj, function( ix, dt ) {
                                   
					i++;
					color = "black";	
					if(ix == "credit") color="red";
                                        table +="<tr>";
                                        table +="<td>"+i+"</td>";
                                        table +="<td><font color='"+color+"'>"+ix+"</font></td>";
                                        table +="<td align='center'><font color='"+color+"'>"+dt+"</font></td>";
                                        table +="</tr>";

                                   });

                                 });

                                 table += "</table>";
                                 $('#cdrinfodata'+winid).html(table);
                            }
                            else {
                                //console.log('ERROR:' +msg.status);
                                // alert("Error: " + msg.status);
                                alert("No data");
                            }
                    },  //Error
                    error: function (xhr, str) {
                          // alert("Error");
                    }
        });
}



function makeRtcpChart(winid, jitter, packetloss, mos) {
	

      jitter.sort(function(a,b) { return a[0] - b[0] } );
      
      packetloss.sort(function(a,b) { return a[0] - b[0] } );
      
      mos.sort(function(a,b) { return a[0] - b[0] } );
      

      var asr1 = [[0, jitter]];
      var asr1Display = jitter;
      //console.log(jitter);

        // registration	

      $.plot($("#rtcpjitterchart"+winid),
                [{
                	data: jitter,
                	lines: { show: true, fill: true },                	
                	//series: { points: { show: true, radius: 3} },                	                        
                	points: { show: true },
                	label: "Jitter MS", 
                	color: "#333"
                }],
           {           	
               xaxis: { mode: "time" }
          });


      $.plot($("#rtcppacketlosschart"+winid),
                [{
                	data: packetloss,
                	lines: { show: true, fill: true},
                	points: { show: true },
                	label: "Packet Loss %", 
                	color: "rgb(200, 20, 30)",
			grid: {
                               hoverable: true,
                                clickable: true
                        }
                }],                
                
                
           {
               xaxis: { mode: "time" }
          });


      $.plot($("#rtcpmoschart"+winid),
                [{
                	data: mos,
                	lines: { show: true, steps: true},
                	label: "MOS", 
			color: "rgb(30, 180, 20)",
                	threshold: {
				below: 3,
				color: "rgb(200, 20, 30)"
			}
                }],                                
           {
               xaxis: { mode: "time" }
          });


}



/* formula from www.voiptroubleshooter.com. Thanks for sharing. */
function calculatePlMos(ppl, codec)
{
	var ie;
	var rate;
	var frame;
	switch ( codec ) {
  	case "g711":
		ie = 0;
		bpl = 10;
		rate = 64000;
		break;
	case "g711plc":
		ie = 0;
		bpl = 34;
		rate = 64000;
		break;
	case "g723153":
		ie = 19;
		bpl = 24;
		rate = 5300;
		frame = 30;
		break;
	case "g723163":
		ie = 15;
		bpl = 20;
		rate = 6300;
		frame = 30;
		break;
	case "g729":
		ie = 10;
		bpl = 17;
		rate = 8000;
		if( frame < 10 ) frame = 10;
		break;
	case "g729a":
		ie = 11;
		bpl = 17;
		rate = 8000;
		if( frame < 10 ) frame = 10;
		break;
	case "ilbc13":
		ie = 10;
		bpl = 28;
		rate = 13300;
		frame = 30;
		break;
	case "ilbc15":
		ie = 12;
		bpl = 28;
		rate = 15200;
		frame = 20;
		break;

	default :
		ie = 0;
		bpl = 10;
		rate = 64000;
	}

	var ie_eff = (ie + (95.0 - ie) * ppl / (ppl + bpl));
	var rlq = (93.2 - ie_eff);
	var mos = String(Math.round(10*(1 + rlq * 0.035 + rlq * (100 - rlq) * (rlq - 60) * 0.000007))/10);
	return mos;
}



//-->
