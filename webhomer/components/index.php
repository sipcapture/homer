<?php

defined( '_HOMEREXEC' ) or die( 'Restricted access' );

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
$nodes = $db->getNodes();
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