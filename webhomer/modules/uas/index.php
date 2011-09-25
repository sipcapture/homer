<?php

include("../../configuration.php");

$date = date("o-m-d");
$dbhost = HOMER_HOST;
$dbuser = HOMER_USER;
$dbpass = HOMER_PW;
$conn = mysql_connect($dbhost, $dbuser, $dbpass) or die ('Error connecting to mysql');
$dbname = HOMER_DB;

$USE_UA = 0;

// CODE START
mysql_select_db($dbname);
$queryUA = mysql_query("SELECT user_agent, count(user_agent) as count FROM sip_capture  WHERE (date > DATE_SUB( NOW(), INTERVAL 12 HOUR) ) GROUP BY user_agent");

$rows = array();

$time = $date = date('H');

// GET USER-AGENTS BREAKDOWN
$row = mysql_fetch_assoc($queryUA);
do{
//$sipUA[] = '{ name: \''.$row['user_agent'].'\', y: '.$row['count'].'}';
$sipUA[] = '{ name: \''.substr($row['user_agent'], 0, 40).'\', y: '.$row['count'].'}';
 } while($row = mysql_fetch_assoc($queryUA));

?>


<!DOCTYPE HTML>
<html>
	<head>
		<meta http-equiv="Content-Type" content="text/html; charset=utf-8">
		<title>HOMER SIP Capture Statistics</title>
		
		<script type="text/javascript" src="/webhomer/js/jquery-1.5.1.min.js"></script>
                <META HTTP-EQUIV="refresh" CONTENT="3600">




<script type="text/javascript">

jQuery(function() {


var chart1 = new Highcharts.Chart({

      chart: {

         renderTo: 'chart3'

      },

      title: {

         text: ''

      },

      legend: {

        enabled: false

      },

      tooltip: {

         formatter: function() {

            if (this.series.name == 'User-Agent') {

		if (this.point.name == '') { this.point.name = 'Undefined'; }

              return '<b>'+ this.point.name +'</b>: '+ this.percentage +' %';


            } else {

              return '<b>' + this.y + ' </b> ' + this.series.name + '';


            }

         }

      },

      series: [{

        name: 'User-Agent',

        type: 'pie',

        data: [
	<?php echo join($sipUA, ', ');?> ],

        dataLabels: {


          enabled: true,

          distance: 20, 

          formatter: function() {

            if (this.point.name == '') {

              return 'Unspecified';

            } else {

              return this.point.name;

            }

          }

        },

        center: [430, 150],

//        size: 300

      }


	]


   }); 
});


  


</script>		




		
	</head>
	<body>
			
		<script type="text/javascript" src="../stats/js/highstock.js"></script>
		<!-- <script type="text/javascript" src="../stats/js/modules/exporting.js"></script> -->
		<legend>User-Agents</legend>
		<div id="chart3" style="width: 95%; margin-left: 17px; float: left; height: 300px"></div>


	</body>
</html>
