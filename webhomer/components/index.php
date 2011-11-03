<?php

defined( '_HOMEREXEC' ) or die( 'Restricted access' );

$task = getVar('task', NULL, '', 'string');
$component = getVar('component', 'search', '', 'string');
$userlevel =  $_SESSION['userlevel'];
$header =  getVar('component', 0, '', 'int');

/* SECURITY LEVEL: 1 - Admin, 2 - Manager, 3 - User, 4 - Guest*/
$components = array("search" => 3, "toolbox" => 3, "statistic" =>3, "admin" => 1);          

/* Disable stats changing security level */
if(detectIE()) {
  $components["statistic"]=0;
  define("IERROR",1);
}
else define("IERROR",0);

#Extra Security check
$security = 0;
foreach($components as $key=>$value) {
	
	if($key == $component) {
		if($userlevel <= $value) $security = 1;
		break;
	}    
}

if($security == 0 ) {
	echo "You don't have permissions to execute this component";
        exit;
}

if($task == "logout") {
    $auth->logout();                                         
    header("Location: index.php\r\n");
    echo "<script>location.href='index.php';</script>\n";
    exit;
}

/* My Nodes */
$mynodeshost = array();
$mynodesname = array();
$nodes = $db->getAliases('nodes');
foreach($nodes as $node) {
        $mynodeshost[$node->id] = $node->host;
        $mynodesname[$node->id] = $node->name;
}

include("components/main/main.php");
$main = new HomerMain();

include("components/".$component."/".$component.".php");
$compononent = new Component();

$main->openPage();

/* DO IT */
$compononent->executeComponent();

/**/
$main->closePage();

?>