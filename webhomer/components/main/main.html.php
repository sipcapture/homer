<?php
/*
 * HOMER Web Interface
 * Homer's homer.html.php
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

defined( '_HOMEREXEC' ) or die( 'Restricted access' );

class HTML_mainhtml {

     function displayStart ($status, $header,$task, $level, $bgcolor) {

        ?>
        <html xmlns="http://www.w3.org/1999/xhtml">
            <head>
            <meta http-equiv="Content-Type" content="text/html; charset=utf-8" />
	    <meta http-equiv="X-UA-Compatible" content="chrome=1" />
            <title>Web Homer - <?php echo $status;?> </title>            
            <link href="styles/core_styles.css" rel="stylesheet" type="text/css" />
            <link href="styles/form.css" rel="stylesheet" type="text/css" />
            <link href="styles/jquery.timeentry.css" rel="stylesheet" type="text/css" />            
            <link type="text/css" href="styles/jquery-ui-1.8.4.custom.css" rel="stylesheet" />	
            <link type="text/css" href="styles/inettuts.css" rel="stylesheet" />
            <link type="text/css" href="styles/inettuts.js.css" rel="stylesheet" />            
            <link rel="icon" href="favicon.ico" />
            </head>
            
            <body style="background-color: <?php echo $bgcolor; ?>;" >          
            <script type="text/javascript">var IER=<?php echo IERROR;?>;</script>
            <script type="text/javascript" src="js/homer.js"></script>
            <!--[if lte IE 8]><script language="javascript" type="text/javascript" src="js/excanvas.min.js"></script><![endif]-->                
            <script src="js/jquery-1.6.4.min.js" type="text/javascript"></script>
            <script type="text/javascript" src="js/jquery-ui-1.8.16.custom.min.js"></script>
            <script type="text/javascript" src="js/jquery.tools.min.js"></script>
	    <script type="text/javascript">
                jQuery(document).ready( function($) {
                  $("[title]").tooltip({ offset: [90, 10], effect: "slide"});
                });
            </script>
<?php

      }

      function displayBackground ($bglogo) {
?>
	<div id="newbg" class="newbg" style="height: auto;"><img id="bgpic" src="images/<?php echo $bglogo; ?>" width="101%" height="auto" /></div>

<?php 
      }
            
      function displayToolBar($datas, $selected, $uptime) {
      
?>
	<div id="banner"> 
		<h1 class="logo"> 
			<a href="index.php" title="Homer Capture Server"><span>Homer</span></a> 
		</h1> 		
		<div id="dock" class="ui-corner-all"> 
			<div class="left"></div> 
			<ul> 
<?php
                            foreach($datas as $key=>$value) {
                            
                                echo "<li ";
                                if($key == $selected) echo "class='selected'";
                                echo ">";
                                echo "<a href='index.php?component=".$key."'>".$value."</a>";
                                echo "</li>";
                            
                            }
?>
				<li style="padding-left: 12px;"> 
					<div style="background: #555; width: 1px; height: 24px; position: absolute; left: 0px;"></div> 
					<a href="index.php?task=logout" title="You logged as: <?php echo $_SESSION['loggedin'];?>">Logout</a> 
				</li> 

				<li <?php if($task!="off") echo "class='selected'"; ?>>
                                    <a href="#"><?php echo $uptime;?></a>
                                </li>

                          </ul> 
			<div class="right"></div> 
		</div> 
<?php

      }

      function displayUserBar($component, $task) {
?>                 
      		
		<div id="navigation"> 
			<div class="left"></div> 
			    <ul id="icons" class="ui-widget ui-helper-clearfix">

<?php if($component == "search" && $task == "result"):?>	
                             <li class="ui-state-default ui-corner-all" style="cursor:pointer;" onClick="javascript:toggleDelta();" id="delta" title="Show delta"><span class="ui-icon ui-icon-alert"></span></li>
                            <li class="ui-state-default ui-corner-all" style="cursor:pointer;" id="setupheader" title="Show Columns"><span class="ui-icon ui-icon-wrench"></span></li>
                            <li class="ui-state-default ui-corner-all" style="cursor:pointer;" onClick="javascript:showSearch();" title="Field Search"><span class="ui-icon ui-icon-search"></span></li>
                            <li class="ui-state-default ui-corner-all" style="cursor:pointer;" onClick="javascript:$('#SIPTable').dataTable().fnFilterClear();" title="Clear Local Filter"><span class="ui-icon ui-icon-cancel"></span></li>
<?php endif; ?>	 
                            <li class="ui-state-default ui-corner-all" style="cursor:pointer;" onClick='javascript:new function(){var c=document.cookie.split(";");for(var i=0;i<c.length;i++){var e=c[i].indexOf("=");var n=e>-1?c[i].substr(0,e):c[i];document.cookie=n+"=;expires=Thu, 01 Jan 1970 00:00:00 GMT";}}();window.location = "index.php";' title="Clear UI / Cookies"><span class="ui-icon ui-icon-trash"></span></li>                            
			                      </ul>
			<div class="right"></div> 
		</div> 
	</div> 
	<div id="content-wrapper"> 
		<div id="content"> 
		<div class="content-top"></div>		
		<div class="content"> 
                <script type="text/javascript"> 
                      var section = "demos/dialog";
                </script> 
<?php
      }

      function displayFormOpen() {
             echo '<form action="index.php" method="POST" name="homer" id="homer">';
       }

      function displayFormClose($component) {
                    
            echo '<input type="hidden" name="task" id="task" value="result">';
            echo '<input type="hidden" name="component" id="component" value="'.$component.'">';
            echo '</form';
      
      }
 
      function displayStop () {
      
            echo '</body></html>';
            
      }
}

?>
