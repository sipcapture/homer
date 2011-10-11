#!/usr/bin/perl

use DBI;

$table = "sip_capture";
$dbname = "homer_db";
$maxparts = 6; #6 days
$newparts = 2; #new partitions in advance for next day. Script must start daily!

#Hours
$maxparts*=96;
$newparts*=96;

my $db = DBI->connect("DBI:mysql:$dbname:localhost:3306", "root", "");

#$db->{PrintError} = 0;

my $query = "SELECT UNIX_TIMESTAMP(CURDATE() - INTERVAL 1 DAY)";
$sth = $db->prepare($query);
$sth->execute();
my ($curtstamp) = $sth->fetchrow_array();
$curtstamp+=0; 

sub return_partcount
{

my $query = "SELECT COUNT(*) FROM INFORMATION_SCHEMA.PARTITIONS"
            ."\n WHERE TABLE_NAME='".$table."' AND TABLE_SCHEMA='".$dbname."'";
$sth = $db->prepare($query);
$sth->execute();
my ($this_partcount) = $sth->fetchrow_array();
$this_partcount--; #do not include pmax

return $this_partcount;
}

my $partcount=&return_partcount;

while($partcount > ($maxparts + $newparts)) {

    $query = "SELECT PARTITION_NAME, MIN(PARTITION_DESCRIPTION)"
             ."\n FROM INFORMATION_SCHEMA.PARTITIONS WHERE TABLE_NAME='".$table."'"
             ."\n AND TABLE_SCHEMA='".$dbname."';";

    $sth = $db->prepare($query);
    $sth->execute();
    my ($minpart,$todaytstamp) = $sth->fetchrow_array();
    $todaytstamp+=0;
    
    #Dont' delete the partition for the current day or for future. Bad idea!
    if($curtstamp <= $todaytstamp) {    
          $partcount = 0;
          next;
    }
           
    #Delete
    $query = "ALTER TABLE ".$table." DROP PARTITION ".$minpart;

    $db->do($query);
    if (!$db->{Executed}) {
           print "Couldn't drop partition: $minpart\n";
           break;
    }

    $partcount=&return_partcount;
}

# < condition
$curtstamp+=(86400);

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
