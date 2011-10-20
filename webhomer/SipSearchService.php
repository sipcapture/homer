<?php

include_once('ISipService.php');
include_once('SipResult.php');


class SipSearchService implements ISipService
{

  protected $connection;

  protected $propertyToColumnMapping = array('id' => 'id',
  					     'date'            => 'date',
                                             'micro_ts'        => 'micro_ts',
                                             'method'	       => 'method',
					     'reply_reason'    => 'reply_reason',
					     'ruri' 	       => 'ruri',
					     'ruri_user'       => 'ruri_user',
                                             'from_user'       => 'from_user',
					     'from_tag'        => 'from_tag',
					     'to_user'         => 'to_user',
					     'to_tag'	       => 'to_tag',
                                             'pid_user'	       => 'pid_user',
					     'contact_user'    => 'contact_user',
					     'auth_user'       => 'auth_user',
					     'callid'	       => 'callid',					     
					     'callidtag'       => 'callid',					     
                                             'callid_aleg'     => 'callid_aleg',
					     'via_1'           => 'via_1',
					     'via_1_branch'    => 'via_1_branch',
					     'cseq'	       => 'cseq',
                                             'diversion'       => 'diversion',
					     'reason'          => 'reason',
					     'content_type'    => 'reason',
					     'authorization'   => 'authorization',
                                             'user_agent'      => 'user_agent',
					     'source_ip'       => 'source_ip',
					     'source_port'     => 'source_port',
					     'destination_ip'  => 'destination_ip',					     
                                             'destination_port'	       => 'destination_port',
					     'contact_ip'       => 'contact_ip',
					     'contact_port'     => 'contact_port',
					     'originator_ip'    => 'originator_ip',
					     'originator_port'  => 'originator_port',
                                             'proto'	        => 'proto',
					     'family'     => 'node',
					     'type'       => 'type',
					     'node'	  => 'node');

  public function __construct($host, $database, $user, $password)
  {
  
    try {
      $this->connection = new PDO("mysql:host=$host;dbname=$database", $user, $password);
    } catch (PDOException $e){
      die($e->getMessage());
    }
  }

  function compare($x, $y)
  {
	if ( $x['micro_ts'] == $y['micro_ts'] )
		return 0;
	else if ( $x['micro_ts'] < $y['micro_ts'] )
		return -1;
	else
		return 1;
  }


  public function getAll($offset, $num, $sort, $sortDirection = 'asc', $isCount = false, $homer)
  {
    $sort = $this->propertyToColumnMapping[$sort];

     $location = $homer->location;

     $skip_keys = array('location','max_records','date', 'from_time', 'to_time', 'unique');
     $ft = date("Y-m-d H:i:s", strtotime($homer->date." ".$homer->from_time));
     $tt = date("Y-m-d H:i:s", strtotime($homer->date." ".$homer->to_time));
     $fhour = date("H", strtotime($homer->date." ".$homer->from_time));
     $thour = date("H", strtotime($homer->date." ".$homer->to_time));
                             
     $j=$thour+1;     
     
     $where = "(`date` >= '$ft' AND `date` <= '$tt' )";              

     $max_records = (int) ($homer->max_records/count($location));
     
      $s=0;
     foreach($homer as $key=>$value) {

	   if(in_array($key, $skip_keys)) continue;

	if(!isset($callwhere)) $callwhere = "(";
	if($key == "callid" && $callid_aleg) $callwhere.=" (";

	$eqlike = preg_match("/%/", $value) ? " like " : " = ";

        //$key = mysql_real_escape_string($key);
        $key = "`".$key."`";
        //$value = "'".mysql_real_escape_string($value)."'";
        $value = "'".$value."'";

        if($s == 1) $callwhere.=" AND ";

        $callwhere.= $key.$eqlike.$value;
	if($key == "callid" && $callid_aleg) $callwhere.= " OR callid_aleg ".$eqlike.$value.") ";

	$s = 1;
    }
                   
    if(isset($callwhere)) $where .= " AND ".$callwhere.")";

    //$node = sprintf("homer_node%02d.", $value);
    // check if we just need a count of results
    if($isCount){
      
      $count=0;
      foreach($location as $value) { 

              $query = "SELECT count(id) as count"
                      ."\n FROM ".HOMER_TABLE
                      ."\n WHERE ". $where;
			
	      $statement = $this->connection->query($query);
	      $result = $statement->fetch();
	      
	      $count += $result['count'];
      }

      return $count;

    } else {

	$results = array();
        $datasort = array();

	foreach($location as $value) {
	
              $query = "SELECT *,'".$value."' as tnode "
                ."\n FROM ".HOMER_TABLE 
                ."\n WHERE ". $where
                ."\n ORDER BY {$sort} {$sortDirection} "
                ."\n limit {$offset}, {$num}";				

	      $statement = $this->connection->query($query);
	      $result = $statement->fetchAll();
              $results = array_merge($results,$result);	      
	}

        //usort($results, 'compare');

	$sipresults = $this->hydrateResults($results, $location);
      
      return $sipresults;
    }
  }




  public function searchAll($search, $columns, $offset, $num, $sort, $sortDirection = 'asc', $isCount = false, $homer, $searchtColumns, $parent)
  {
    $whereSqlParts = array();

     $location = $homer->location;

     $skip_keys = array('location','max_records','date', 'from_time', 'to_time','unique');
     $ft = date("Y-m-d H:i:s", strtotime($homer->date." ".$homer->from_time));
     $tt = date("Y-m-d H:i:s", strtotime($homer->date." ".$homer->to_time));
     $fhour = date("H", strtotime($homer->date." ".$homer->from_time));
     $thour = date("H", strtotime($homer->date." ".$homer->to_time));

     $j=$thour+1;     
     
     $where = " AND (`date` >= '$ft' AND `date` <= '$tt' )";              

     $max_records = (int) ($homer->max_records/count($location));
     
      $s=0;
     foreach($homer as $key=>$value) {

	if(in_array($key, $skip_keys)) continue;

	if(!isset($callwhere)) $callwhere = "(";
	if($key == "callid" && $callid_aleg) $callwhere.=" (";

	$eqlike = preg_match("/%/", $value) ? " like " : " = ";

        //$key = mysql_real_escape_string($key);
        $key = "`".$key."`";
        //$value = "'".mysql_real_escape_string($value)."'";
        $value = "'".$value."'";

        if($s == 1) $callwhere.=" AND ";

        $callwhere.= $key.$eqlike.$value;
	if($key == "callid" && $callid_aleg) $callwhere.= " OR callid_aleg ".$eqlike.$value.") ";

	$s = 1;
    }
                   
    if(isset($callwhere)) $where .= " AND ".$callwhere.")";

    if(empty($searchtColumns)) {    
        foreach($columns as $column){
          // get db column name
          $columnName = $this->propertyToColumnMapping[$column];
          $whereSqlParts[] = "{$column} LIKE '{$search}%'";           
      }
    }
    else {
      foreach($searchtColumns as $key=>$value){
      // get db column name      
       $columnName = $parent->getColumnIndexByNumber($key);
       $eqlike = preg_match("/%/", $value) ? " like " : " = ";
       $whereSqlParts2[] = "{$columnName} {$eqlike} '{$value}'";
      }
    }

    // build where clause for all columns we are searching against
    $whereSql = implode($whereSqlParts, ' OR ');
    $whereSql .= implode($whereSqlParts2, ' AND ');

    //$node = sprintf("homer_node%02d.", $value);

    // check if we just need a count of results
    if($isCount){

      $count=0;
      foreach($location as $value) {

              $query = "SELECT count(id) as count"
                      ."\n FROM ".HOMER_TABLE
                      ."\n WHERE ({$whereSql})". $where;                                                                                        

              $statement = $this->connection->query($query);
              $result = $statement->fetch();

              $count += $result['count'];
      }

      return $count;

    } else {

      $sort = $this->propertyToColumnMapping[$sort];

      $results = array();
      $datasort = array();

      foreach($location as $value) {

              $query = "SELECT *,'".$value."' as tnode "
                                ."\n FROM ".HOMER_TABLE
                                ."\n WHERE ({$whereSql}) ". $where
                                ."\n ORDER BY {$sort} {$sortDirection} "
                                ."\n LIMIT {$offset}, {$num}"
                                ;

              $statement = $this->connection->query($query);
              $result = $statement->fetchAll();
              $results = array_merge($results,$result);

        }

        //usort($results, 'compare');

        $sipresults = $this->hydrateResults($results, $location);

      return $sipresults;
    }
  }
  
  /**
   * Hydrate a db result set into an array of SipResult objects
   * 
   */
  protected function hydrateResults($results, $location)
  {
    $browsers = array();    
	// convert result set into array of SipResult objects
    foreach($results as $result){
    
      $sipresults[] = new SipResult($result, $location);
    }
    
    return $sipresults;
  }
}
