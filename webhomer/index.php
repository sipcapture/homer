<?php
//instantiate if needed
include("class.db.php");
$log = new homer();
$log->encrypt = true; //set encryption
if($_REQUEST['action'] == "login"){
	if($log->login("logon", $_REQUEST['username'], $_REQUEST['password']) == true){
		//do something on successful login	
		header("Location: homer.php\n\n");
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
<form action="index.php" method="post">
<table width="400" border="0" align="center" cellpadding="0" cellspacing="0"  class="login">
  <tr>
    <td colspan="3" class="login"><strong>Web Homer access</strong></td>
  </tr>
  <tr>
    <td height="50" colspan="3" class="login">Please use your login and password to login</td>
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
