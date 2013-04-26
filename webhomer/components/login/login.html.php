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

defined( '_HOMEREXEC' ) or die( 'Restricted access' );

class HTML_login {

        function displayLoginForm () {

?>
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">
<html xmlns="http://www.w3.org/1999/xhtml">
<head>
<meta http-equiv="Content-Type" content="text/html; charset=UTF-8" />
<title>Homer - SIP Capture for the masses</title>
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

<?php 
	if(CONFIG_VERSION != "1.0.10") echo "<h2><font color='red'>Your CONFIG FILE is outdated!</font></h2>"; 
 	if (!is_writable(PCAPDIR)) {echo "<h2><font color='red'>Your TMP directory is not writeable!</font></h2>";}
?>

<form action="index.php" method="post">
<table width="400" border="0" align="center" cellpadding="0" cellspacing="0"  class="login">
  <tr>
	<td colspan="1" class="login" align="center">
		<img src="images/homerlogo.png" alt="Homer Web Access">
		<br><font size=-2> webHomer <?php echo WEBHOMER_VERSION;?></font>
	</td>
  </tr>

<?php if(defined('AUTHENTICATION_TEXT')): ?>
  <tr>
    <td height="50" colspan="1" class="login" align="center" id="logintext">
<?php echo AUTHENTICATION_TEXT; ?>
  </tr>
<?php endif; ?>  

  <tr align="center">
    <td width="227" height="30"><input name="username" type="text" id="username" placeholder="Login" size="30" class="ui-input ui-widget ui-state-default ui-corner-all" /></td>
  </tr>
  <tr align="center">
    <td height="30"><input name="password" type="password" id="password" size="30" placeholder="Password" class="ui-input ui-widget ui-state-default ui-corner-all" /></td>
  </tr>
  <tr align="right">
    <td height="50" class="login"><input type="submit" name="submit" id="submit" value="login" class="ui-button ui-widget ui-state-default ui-corner-all" /></td>
  </tr>
</table>
<input type="hidden" name="task" value="do">
<input type="hidden" name="component" value="login">
</form>
</body>
</html>
<?php
	}
}
?>
