<?php

include("../../configuration.php");

$date = date("o-m-d");
$dbhost = HOMER_HOST;
$dbuser = HOMER_USER;
$dbpass = HOMER_PW;
$conn = mysql_connect($dbhost, $dbuser, $dbpass) or die ('Error connecting to mysql');
$dbname = HOMER_DB;


$USE_100 = 1;
$USE_401 = 1;

// CODE START
mysql_select_db($dbname);
$query = mysql_query("SELECT UNIX_TIMESTAMP(date), count(id) as count FROM sip_capture  WHERE (`date` > DATE_SUB( NOW(), INTERVAL 12 HOUR) )  GROUP BY HOUR( `date` ) ASC LIMIT 11") or die  (mysql_error());
$query401 = mysql_query("SELECT UNIX_TIMESTAMP(date), count(id) as count FROM sip_capture  WHERE (`date` > DATE_SUB( NOW(), INTERVAL 12 HOUR) ) AND method = '401'  GROUP BY HOUR( `date` ) ASC LIMIT 11") or die  (mysql_error());
$query100 = mysql_query("SELECT UNIX_TIMESTAMP(date), count(id) as count FROM sip_capture  WHERE (`date` > DATE_SUB( NOW(), INTERVAL 12 HOUR) ) AND (method = 100 OR method = 180 OR method = 183 OR method = 200) GROUP BY HOUR( `date` ) ASC LIMIT 11") or die  (mysql_error());
//$queryREG = mysql_query("SELECT UNIX_TIMESTAMP(date), count(id) as count FROM sip_capture  WHERE (method="200" and cseq like "%REGISTER") and (`date` > DATE_SUB( NOW(), INTERVAL 12 HOUR) )  GROUP BY HOUR( date )")  or die  (mysql_error());
//$query100 = mysql_query("SELECT UNIX_TIMESTAMP(date), count(id) as count FROM sip_capture  WHERE (`date` > DATE_SUB( NOW(), INTERVAL 12 HOUR) ) AND method = 200 OR method = 180 OR method = 183 GROUP BY HOUR( `date` ) ASC LIMIT 11") or die  (mysql_error());
$queryTOT = mysql_query("SELECT count(id) as count FROM sip_capture;"); 

$rows = array();


// GET TOTAL CAPTURE BREAKDOWN
$row = mysql_fetch_assoc($query);
//$json_secondes = array();
//$json_date = array();
do{
//$secondes[] = $row['count'];
$secondes[] = '['.$row['UNIX_TIMESTAMP(date)'].'000, '.$row['count'].']';
}
while($row = mysql_fetch_assoc($query));

// GET AUTH FAILURES BREAKDOWN
if ( $USE_401 == 1 ) {
$row = mysql_fetch_assoc($query401);
do{
//$sip401[] = $row['count'];
$sip401[] = '['.$row['UNIX_TIMESTAMP(date)'].'000, '.$row['count'].']';
 } while($row = mysql_fetch_assoc($query401));
}

// GET SIP SESSIONS BREAKDOWN
if ( $USE_100 == 1 ) {
$row = mysql_fetch_assoc($query100);
do{
//$sip100[] = $row['count'];
$sip100[] = '['.$row['UNIX_TIMESTAMP(date)'].'000, '.$row['count'].']';
 } while($row = mysql_fetch_assoc($query100));
}

// GET CAPTURE GRANDTOTAL
$row = mysql_fetch_assoc($queryTOT);
do{
$sipTOT = $row['count'];
}
while($row = mysql_fetch_assoc($queryTOT));


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

         renderTo: 'chart1'

      },

      title: {

         text: 'Homer Stats'

      },

      legend: {

        enabled: false

      },

      xAxis: [{

	type: 'datetime'

//         categories: ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 

//            'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

      }],

      yAxis: [{ // Primary yAxis

         labels: {

            formatter: function() {

               return this.value +' /hr';

            },

            style: {

               color: '#DDDF0D',
	      	    fontWeight: 'bold',
	            fontSize: '12px',
	            fontFamily: 'Trebuchet MS, Verdana, sans-serif'

            },

            align: 'left',

            x: 0,

            y: -2

         },

          showFirstLabel: false,

         title: {

            text: 'Packets/Hour',

            style: {

               color: '#89A54E'

            }

         }

      }, { // Secondary yAxis

         title: {

            text: 'Sessions/hour',

            style: {

               color: '#4572A7'

            },

	 max: '10000',

         },

         labels: {

            align: 'right',

            x: 0,

            y: -2,

            formatter: function() {

               return this.value +'/hr';

            },

            style: {

               color: '#4572A7',
	 	    fontWeight: 'bold',
	            fontSize: '12px',
	            fontFamily: 'Trebuchet MS, Verdana, sans-serif'

            }

         },

        showFirstLabel: false,

         opposite: true

      }],

      tooltip: {

         formatter: function() {

            if (this.series.name == 'Calls') {

              return '<b>Sessions:</b><br> '+ this.y + '/hour';

            }  else  if (this.series.name == 'User-Agent') {

              return false;


            } else {

              return '<b>' + this.y + ' </b> ' + this.series.name + '';


            }

         }

      },

      series: [{

         name: 'Calls',

         color: '#4572A7',

//         type: 'column',
         type: 'spline',

//         yAxis: 1,

	data :  [<?php echo join($sip100, ', ');?>]
      

      }, {

         name: 'Auth Failures',

         color: '#FF72A7',

         type: 'spline',

//         yAxis: 1,

        data :  [<?php echo join($sip401, ', ');?>]


      }, {

         name: 'Packets',

         color: '#DDDF0D',

         type: 'spline',

         data: [<?php echo join($secondes, ', ');?>]

      }]

   });
   });

  


</script>		




		
	</head>
	<body>
			
		<script type="text/javascript" src="./js/highstock.js"></script>
		<!-- <script type="text/javascript" src="./js/modules/exporting.js"></script> -->
		<legend>Total Packets: <?php echo $sipTOT; ?></legend>
		<div id="chart1" style="width: 95%; margin-left: 17px; float: left; height: 300px"></div>

	</body>
</html>
