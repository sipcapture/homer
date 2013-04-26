<?php
/*
 * HOMER Web Interface
 * Homer's ajax.php
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

define('_HOMEREXEC', "1");

/* MAIN CLASS modules */ 
include("class/index.php");
      
// register the DataTable autoloader
include('DataTable/Autoloader.php');
spl_autoload_register(array('DataTable_Autoloader', 'autoload'));

// include the Demo DataTable class
include_once('SipDataTable.php');

// build a Browser Service object based on the type that was selected
include_once('SipSearchService.php');
                         
$sipService = new SipSearchService($db->hostname_homer);
  
// instatiate new DataTable
$table = new SipDataTable();

$table->setBrowserService($sipService);

$tmpdata = json_decode($_REQUEST['data'], true);

$data = array();
foreach($tmpdata as $k=>$v) {
    $data[$v['name']] = $v['value'];
}

$request = new DataTable_Request();
$request->fromPhpRequest($data);

// render the JSON data string
echo $table->renderJson($request);
