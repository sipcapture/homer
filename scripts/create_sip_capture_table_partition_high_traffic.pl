#!/usr/bin/perl

$noArgs = $#ARGV + 1;

if ($noArgs != 2) {
    print "Usage: ./create_sip_capture_table_partition_high_traffic.pl <Date to create e.g. 2011-10-06> <Time Zone e.g. America/Los_Angeles>\n";
    exit;
}

$dateinput = $ARGV[0];
$timezone = $ARGV[1];

my $ckdate=&isvaliddate($dateinput);

if($ckdate<1) {
   die "Invalid Date Input\n";
}

use DBI;
use DateTime;

$table = "sip_capture";
$dbname = "homer_db";
$maxparts = 6; #6 days
$newparts = 2; #new partitions for next 2 days. Script must start daily!

#Every 15 minutes apart = 96 per day
$maxparts*=96;
$newparts*=96;

sub isvaliddate {
  my $input = shift;
  if ($input =~ m!^((?:19|20)\d\d)[- /.](0[1-9]|1[012])[- /.](0[1-9]|[12][0-9]|3[01])$!) {
   #  At this point, $1 holds the year, $2 the month and $3 the day of the date entered
    if ($3 == 31 and ($2 == 4 or $2 == 6 or $2 == 9 or $2 == 11)) {
      return 0; # 31st of a month with 30 days
    } elsif ($3 >= 30 and $2 == 2) {
      return 0; # February 30th or 31st
    } elsif ($2 == 2 and $3 == 29 and not ($1 % 4 == 0 and ($1 % 100 != 0 or $1 % 400 == 0))) {
      return 0; # February 29th outside a leap year
    } else {
      $idt = DateTime->new( year => $1, month => $2, day => $3, hour => 0, minute => 0, second => 0, nanosecond => 0, time_zone => $timezone );
      return 1; # Valid date
    }
  } else {
    return 0; # Not a date
  }
}

my $db = DBI->connect("DBI:mysql:$dbname:localhost:3306", "root", "");

#$db->{PrintError} = 0;

my $query = "SELECT UNIX_TIMESTAMP(CURDATE() - INTERVAL 1 DAY)";
$sth = $db->prepare($query);
$sth->execute();
my ($curtstamp) = $sth->fetchrow_array();
$curtstamp+=0; 

$curtstamp = $idt->epoch;

my $sth = $db->do("
CREATE TABLE IF NOT EXISTS `".$table."` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `date` timestamp NOT NULL DEFAULT '0000-00-00 00:00:00',
  `micro_ts` bigint(18) NOT NULL DEFAULT '0',
  `method` varchar(50) NOT NULL DEFAULT '',
  `reply_reason` varchar(100) NOT NULL,
  `ruri` varchar(200) NOT NULL DEFAULT '',
  `ruri_user` varchar(100) NOT NULL DEFAULT '',
  `from_user` varchar(100) NOT NULL DEFAULT '',
  `from_tag` varchar(64) NOT NULL DEFAULT '',
  `to_user` varchar(100) NOT NULL DEFAULT '',
  `to_tag` varchar(64) NOT NULL,
  `pid_user` varchar(100) NOT NULL DEFAULT '',
  `contact_user` varchar(120) NOT NULL,
  `auth_user` varchar(120) NOT NULL,
  `callid` varchar(100) NOT NULL DEFAULT '',
  `callid_aleg` varchar(100) NOT NULL DEFAULT '',
  `via_1` varchar(256) NOT NULL,
  `via_1_branch` varchar(80) NOT NULL,
  `cseq` varchar(25) NOT NULL,
  `diversion` varchar(256) NOT NULL,
  `reason` varchar(200) NOT NULL,
  `content_type` varchar(256) NOT NULL,
  `authorization` varchar(256) NOT NULL,
  `user_agent` varchar(256) NOT NULL,
  `source_ip` varchar(50) NOT NULL DEFAULT '',
  `source_port` int(10) NOT NULL,
  `destination_ip` varchar(50) NOT NULL DEFAULT '',
  `destination_port` int(10) NOT NULL,
  `contact_ip` varchar(60) NOT NULL,
  `contact_port` int(10) NOT NULL,
  `originator_ip` varchar(60) NOT NULL DEFAULT '',
  `originator_port` int(10) NOT NULL,
  `proto` int(5) NOT NULL,
  `family` int(1) DEFAULT NULL,
  `rtp_stat` varchar(256) NOT NULL,
  `type` int(2) NOT NULL,
  `node` varchar(125) NOT NULL,
  `msg` text NOT NULL,
  PRIMARY KEY (`id`,`date`),
  KEY `ruri_user` (`ruri_user`),
  KEY `from_user` (`from_user`),
  KEY `to_user` (`to_user`),
  KEY `pid_user` (`pid_user`),
  KEY `auth_user` (`auth_user`),
  KEY `callid_aleg` (`callid_aleg`),
  KEY `date` (`date`),
  KEY `callid` (`callid`),
  KEY `method` (`method`),
  KEY `source_ip` (`source_ip`),
  KEY `destination_ip` (`destination_ip`)
) ENGINE=MyISAM DEFAULT CHARSET=latin1
PARTITION BY RANGE ( UNIX_TIMESTAMP(`date`)) (PARTITION pmax VALUES LESS THAN MAXVALUE ENGINE = MyISAM);
");

#Create new partitions 
for(my $i=0; $i<$newparts; $i++) {

    $oldstamp = $curtstamp;
    $curtstamp+=900;
    
    ($sec,$min,$hour,$mday,$mon,$year,$wday,$yday,$isdst) = localtime($oldstamp);

    my $newpartname = sprintf("p%04d%02d%02d%02d%02d",($year+=1900),(++$mon),$mday,$hour,$min);    
    
    $query = "SELECT COUNT(*) "
             ."\n FROM INFORMATION_SCHEMA.PARTITIONS WHERE TABLE_NAME='".$table."'"
             ."\n AND TABLE_SCHEMA='".$dbname."' AND PARTITION_NAME='".$newpartname."'"
             ."\n AND PARTITION_DESCRIPTION = '".$curtstamp."'";
             
    $sth = $db->prepare($query);
    $sth->execute();
    my ($exist) = $sth->fetchrow_array();
    $exist+=0;
    
    if(!$exist) {
    
        $query = "ALTER TABLE ".$table." REORGANIZE PARTITION pmax INTO (PARTITION ".$newpartname
             ."\n VALUES LESS THAN (".$curtstamp.") ENGINE = MyISAM, PARTITION pmax VALUES LESS THAN MAXVALUE ENGINE = MyISAM)";

        $db->do($query);
        if (!$db->{Executed}) {
             print "Couldn't add partition: $newpartname\n";
        }

    }    
}
