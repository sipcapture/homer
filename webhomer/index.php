<?php
/*
 * HOMER Web Interface
 * Homer's index.php
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

define(_HOMEREXEC, 1);
define(SKIPAUTH, 1);

/* MAIN CLASS modules */
include("class/index.php");

if($_REQUEST['action'] == "login"){
	if($auth->login($_REQUEST['username'], $_REQUEST['password']) == true){
		//do something on successful login	
		header("Location: homer.php?component=search\n\n");
		exit;
	}else{
		//do something on FAILED login	
		echo "<font color='red'>Bad Passwort!</font>";		
	}
}
?>
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">
<html xmlns="http://www.w3.org/1999/xhtml">
<head>
<meta http-equiv="Content-Type" content="text/html; charset=UTF-8" />
<title>Homer - because sip capturing make sense</title>
<style type="text/css">
<!--
table.login {

	background-color: #FFF;
	border: 1px solid #666;
}

td.login {
padding: 8px;
}
body,td,th {
	font-family: Arial, Helvetica, sans-serif;
	font-size: 12px;


}

body {

	background-color: #FFF;
	padding-top: 200px;
}
-->
</style>
</head>

<body>

<?php if(CONFIG_VERSION != "1.0.3") echo "<h2><font color='red' align='center'>Your CONFIG FILE is outdate.</font></h2"; ?>

<form action="index.php" method="post">
<table width="400" border="0" align="center" cellpadding="0" cellspacing="0"  class="login">
  <tr>
	<td colspan="3" class="login">
		<img src="images/homerlogo.png" alt="Homer Web Access">
	</td>
  </tr>
  <tr>
    <td height="50" colspan="3" class="login">Please use your Homer login and password</td>
  </tr>
  <tr>
    <td width="150" height="30" align="right">Login:</td>
    <td width="21" height="30">&nbsp;</td>
    <td width="227" height="30"><input style="width:150px;" name="username" type="text" id="username" size="24" /></td>
  </tr>
  <tr>
    <td height="30" align="right">Password:</td>
    <td width="21" height="30">&nbsp;</td>
    <td height="30"><input style="width:150px;" name="password" type="password" id="password" size="24" /></td>
  </tr>
  <tr>
    <td>&nbsp;</td>
    <td width="21">&nbsp;</td>
    <td height="50" class="login"><input type="submit" name="submit" id="submit" value="Login" /></td>
  </tr>
</table>
<input type="hidden" name="action" value="login">
</form>
</body>
</html>
