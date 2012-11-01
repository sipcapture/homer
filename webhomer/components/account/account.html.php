<?php
/*
 * HOMER Web Interface
 * Homer's Account
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


class HTML_Account {


	static function displayAccount() {

?>
<form action="index.php" method="POST" name="account" id="account">	
	         <script type="text/javascript">
		function reset_pwd(){
			if (document.getElementById('password').value != document.getElementById('retype-pwd')){
          	   
			}
			else {
				document.account.submit();
			}
		}
	         </script>
  <div id="columns">
        <ul id="column1" class="column" style="width: 30%;">
        </ul>

        <ul id="column2" class="column" style="width: 40%;">
            <li class="widget color-yellow" id="widget-password">
                <div class="widget-head">
                    <h3>Change Password</h3>
                </div>
                         			
                <div id = "pwd-data" class="widget-content">
          <br>
					 <table class="bodystyle" cellspacing="1"  height="100" width="95%">	
						 <tr>
						 	<td width="150" class="tablerow_two">
				            	<label title="Old Password">Old Password</label>	
				            </td>
				            <td>
				            	<input type="password" id= "old-pwd" name = "old-pwd" class="textfieldstyle2" size="30"  />
				
				            </td>
						 </tr>
						 <tr>
						 	<td></td>
						 	<td id="pwdtext">
						 	Please enter the same value:
						 	</td>
						 </tr>
						 <tr>
				         	<td width="150" class="tablerow_two">
				            	<label title="New Password">New Password</label>	
				            </td>
				            <td>
				            	<input type="password" id="password" name ="password" class="textfieldstyle2" size="30"  />
							</td>
				         </tr>
				         <tr>
				         	<td width="150" class="tablerow_two">
				            	<label title="Retype Password">Retype Password</label>
				
				            </td>                             
				            <td>
				            	<input type="password" id= "retype-pwd" name = "retype-pwd" class="textfieldstyle2" size="30"  />
				
				            </td>
				         </tr>
				         <tr>
						 	<td>	
							</td>
				
				            <td>
								<input type="submit" style="background: transparent;" title="Reset Password" onclick="reset_pwd();"
						 		value="Reset" role="button"  class="ui-button ui-widget ui-state-default ui-corner-all">
				
				            </td>
						</tr>
			      	</table>
				</div>
     		</ul>   
</div>
<input type="hidden" name="task" id="task" value="resetpwd">
<input type="hidden" name="component" id="component" value="account">
</form>
<?php
	}
}
?>