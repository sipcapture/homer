#!/usr/bin/perl

use DBI;
use POSIX;

$version = "0.5.1";
$mysql_dbname = "homer_statistic";
$mysql_data = "homer_data";
$mysql_user = "root";
$mysql_password = "password";
$mysql_host = "localhost";

$interval = $ARGV[0] // "300"; #Minutes
$proto = "hepall";

if($interval eq "help")
{
   print "\nh5_stats_dump.pl [interval]: \n\ninterval: interval in minutes. By default: 300 min.\n\nI.e.: Get stats for last 100 minutes:  h5_stats_dump.pl 100\n";
   print "\n\n";
   exit(0);
}

$interval+=0;

$db = DBI->connect("DBI:mysql:$mysql_dbname:$mysql_host:3306", $mysql_user, $mysql_password);

my $query = "SELECT (NOW() - INTERVAL $interval MINUTE), NOW();"; 
$sth = $db->prepare($query);
$sth->execute();
my @ref = $sth->fetchrow_array();
$mints = $ref[0];
$maxts = $ref[1];

#$step = "60";
#if($interval > 10600) { $step = "86400"; }
#elsif($interval > 3600) { $step = "3600"; 
$step = $interval;
$table = "stats_method";
$global = 0;
print "\nStep resolution: $step minutes.  Report: $mints - $maxts\n\n";
print "Report MAX Packets rate:\r\n";
 
$maxhep = 0;
$value = 0;

$query = "SELECT from_date, total from $table WHERE from_date BETWEEN NOW()- INTERVAL $interval MINUTE AND NOW() AND method = 'TOTAL' order by total DESC limit 1";
$sth = $db->prepare($query);
$sth->execute();
while(my @ref = $sth->fetchrow_array())
{   
   
   $statstime = $ref[0];
   $value = $ref[1];
}

$global+=$value;

$rate = $value / 300;

printf("[$statstime]    Packets: %-11s Rate: %.2f pps\n", $value, $rate);
$maxhep = $value;

#TOTAL SIP
print "\nSIP message report:\n";
$table = "stats_method";
$query = "SELECT * FROM $table WHERE from_date = '".$statstime."' AND method!='TOTAL' order by total DESC";

$logshep = 0;

$sth = $db->prepare($query);
$sth->execute();
while(my @ref = $sth->fetchrow_array())
{

   my $id = $ref[0];
   my $statstime = $ref[1];
   my $countername = $ref[3];
   my $auth = $ref[4];
   my $totag = $ref[6];
   my $value = $ref[7];
   my $rate = $value / 300;

   printf("[$statstime]\t Counter: [%-10s A:%s, T:%s]   Packets: %-10s  Rate: %.2f pps\n", $countername, $auth, $totag, $value, $rate);

}

#TOTAL RTCP
print "\nRTCP message report:\n";
$table = $mysql_data.".rtcp_capture";
$query = "SELECT count(*) FROM $table WHERE date BETWEEN '".$statstime."' AND '".$statstime."' + INTERVAL 5 minute";

$sth = $db->prepare($query);
$sth->execute();
while(my @ref = $sth->fetchrow_array())
{

   my $total = $ref[0];
   my $rate = $total / 300;

   printf("[$statstime]\t Counter: [RTCP]   Packets: %-10s  Rate: %.2f pps\n", $total, $rate);
   
   $global+=$total;
}


#TOTAL LOG
print "\nLog message report:\n";
$table = $mysql_data.".logs_capture";
$query = "SELECT count(*) FROM $table WHERE date BETWEEN '".$statstime."' AND '".$statstime."' + INTERVAL 5 minute";

$sth = $db->prepare($query);
$sth->execute();
while(my @ref = $sth->fetchrow_array())
{

   my $total = $ref[0];
   my $rate = $total / 300;

   printf("[$statstime]\t Counter: [log]   Packets: %-10s  Rate: %.2f pps\n", $total, $rate);
   
   $global+=$total;
}


print "\nAprox total messages:\n";
my $rate = $global / 300;
   
printf("[$statstime]\t TOTAL Packets: %-10s  Rate: %.2f pps\n", $global, $rate);


