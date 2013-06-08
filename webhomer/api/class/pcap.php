<?php
/*
 * HOMER Web Interface
 * Homer's index.php
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


defined( '_HOMEREXEC' ) or die( 'Restricted access' );

class pcap_hdr {
   public $magic; 	// 4 bits
   public $version_major; //2
   public $version_minor; //2
   public $thiszone; //4
   public $sigfigs;  //4
   public $snaplen;  //4
   public $network; //4
}

class pcaprec_hdr {
    public $ts_sec;  //4
    public $ts_usec; //4
    public $incl_len; //4
    public $orig_len; //4
}

// ethernet packet header - 14 bytes
class ethernet_header {
	public $dest_mac; 	// 48 bit 
	public $src_mac;	// 48 bit 
	public $type;	// 16 bit 
}

class ipv4_packet {
	public $ver_len;	// 4 bits
	public $tos;		// 8 bits
	public $total_len;	// 16 bits
	public $ident;		// 16 bits
	public $fl_fr;		// 4 bits
	public $ttl;		// 8 bits
	public $proto;		// 8 bits
	public $checksum;	// 16 bits
	public $src_ip;		// 32 bits
	public $dst_ip;		// 32 bits
	public $options;
}

class udp_header {
       public $src_port; //16 bits
       public $dst_port;  //16 bits
       public $length;    //16 bits
       public $checksum;  //16 bits
}

class size_hdr {
      public $ethernet=14;
      public $ip = 20;
      public $ip6 = 20;
      public $udp = 8;
      public $tcp = 20;
      public $data = 0;
      public $total = 0;
}



class Export {

       function checksum($data) 
       { 
            if( strlen($data)%2 ) $data .= "\x00";      
            $bit = unpack('n*', $data); 
            $sum = array_sum($bit);      
            while ($sum >> 16) 
               $sum = ($sum >> 16) + ($sum & 0xffff); 
            $sum = ~$sum;
            $sum = $sum & 0xffff;
            return $sum;
       } 
       
       function setDB($db) {
            $this->db =  $db;
	}


       function generatePcap($param, $text) 
       {

               global $mynodes;
               
               $mydb = $this->db;
               $reqdata = (array) $param;
               
               $size = new size_hdr();
               //Write PCAP HEADER 
               $pcaphdr = new pcap_hdr();
               $pcaphdr->magic = 2712847316;
               $pcaphdr->version_major = 2;
               $pcaphdr->version_minor = 4;
               $pcaphdr->thiszone = 0;
               $pcaphdr->sigfigs = 0;
               $pcaphdr->snaplen = 102400;
               $pcaphdr->network = 1;

               $buf="";
               $pcap_packet = pack("lssllll", $pcaphdr->magic, $pcaphdr->version_major, 
                      $pcaphdr->version_minor, $pcaphdr->thiszone, 
                      $pcaphdr->sigfigs, $pcaphdr->snaplen, $pcaphdr->network);

               //Ethernet header
               $eth_hdr = new ethernet_header();
               $eth_hdr->dest_mac = "020202020202";
               $eth_hdr->src_mac = "010101010101";
               $eth_hdr->type = "0800";
               $ethernet = pack("H12H12H4", $eth_hdr->dest_mac, $eth_hdr->src_mac, $eth_hdr->type);

               //Temporally tnode == 1
               $tnode=1;
               $option = array(); //prevent problems

               // Check if Table is set
               if (!isset($table)) { $table="sip_capture"; }

               //$cid="1234567890";

               //Empty result
               if(count($reqdata) == 0) {               
                    return;
               }

               // Get Variables
               $b2b = getVar('b2b', NULL, $reqdata, 'string');
               $from_user = getVar('from_user', NULL, $reqdata, 'string');
               $to_user = getVar('to_user', NULL, $reqdata, 'string');
               $limit = getVar('limit', 100, $reqdata, 'int');
               // Get time & date if available
               $flow_from_date = getVar('from_date', NULL, $reqdata, 'string');
               $flow_to_date = getVar('to_date', NULL, $reqdata, 'string');
               $flow_from_time = getVar('from_time', NULL, $reqdata, 'string');
               $flow_to_time = getVar('to_time', NULL, $reqdata, 'string');
               $unique = getVar('unique', 0, $reqdata, 'int');
               $location = getVar('location', array(0), $reqdata, 'array');
               $cid_array = getVar('cid', NULL, $reqdata, 'array');
               
               
               if(is_array($cid_array)) $cid = $cid_array[0];
               else $cid = $cid_array;

               if(!$text) $buf=$pcap_packet;

               /* HOMER DB */
               if(!$mydb->dbconnect_homer(isset($mynodes[$location[0]]) ? $mynodes[$location[0]] : NULL))
               {
                     //No connect;
                      return array();
               }

               if(BLEGDETECT == 1) $b2b = 1;

               if (isset($flow_to_date, $flow_from_time, $flow_to_time))
               {
                       $ft = date("Y-m-d H:i:s", strtotime($flow_from_date." ".$flow_from_time));
                       $tt = date("Y-m-d H:i:s", strtotime($flow_to_date." ".$flow_to_time));
                       $where = "( `date` BETWEEN '$ft' AND '$tt' )";
               }

               /* Prevent break SQL */
               if(isset($where)) $where.=" AND ";

               // Build Search Query
               if(isset($cid_array)) 
               {

                    /* CID */
                    $fileid="CID_".$cid;
                    /* Detect second B-LEG ID */
                    if($b2b) 
                    {
                          if(BLEGCID == "x-cid") 
                          {
                               foreach($location as $value) 
                               {
                                     $mydb->dbconnect_homer(isset($mynodes[$value]) ? $mynodes[$value] : NULL);
                                     foreach ($mynodes[$value]->dbtables as $tablename)
                                     {
                                            $query = "SELECT callid FROM ".$tablename." WHERE ".$where." callid_aleg='".$cid."'";
                                            $cid_aleg = $mydb->loadResult($query);
                                            $cid_array[] = $cid_aleg;
                                            if (!empty($cid_aleg)){
                                                 break 2;
                                            }
                                     }
                               }
                          }
                          else if (BLEGCID == "b2b") 
                          { 
                                if (!preg_match("/%/", $value.BLEGTAIL))
                                {    
                                     /*mysql wildcard % not supported*/
                                     $cid_aleg = $cid.BLEGTAIL;
                                     $cid_array[] = $cid_aleg;
                                }
                          }
                    }	         
               } 
               else if(isset($from_user)) 
               {
                    $fileid="FROM_".$from_user."_".mt_rand();
                    $where .= "( from_user = '".$from_user."'";
                    if(isset($to_user)) { $where .= " OR to_user='".$to_user."') AND "; } 
                    else {  $where .= ") AND ";}
               } 
               else if(isset($to_user)) 
               {
                    $fileid="TO_".$to_user."_".mt_rand();
                    $where .= "( to_user = '".$to_user."') AND ";
               }

               if(!isset($limit)) { $limit = 100; }

               $localdata=array();
               $results = array();

               foreach($location as $value) 
               {

                   $mydb->dbconnect_homer(isset($mynodes[$value]) ? $mynodes[$value] : NULL);
                   $tnode = "'".$value."' as tnode";
                   if($unique) $tnode .= ", MD5(msg) as md5sum";
                   foreach ($mynodes[$value]->dbtables as $tablename)
                   {
                         foreach($cid_array as $cid) 
                         {	            
                               $local_where = $where." ( callid = '".$cid."' )";                               	            
                               $query = "SELECT *, ".$tnode."\n FROM ".$tablename
                                     ."\n WHERE ".$local_where." order by micro_ts ASC limit ".$limit;	
                               $result = $mydb->loadObjectArray($query);
	
                               // Check if we must show up only UNIQ messages. No duplicate!
                               //only unique
                               if($unique) 
                               {
                                    foreach($result as $key=>$row) 
                                    {
                                         if(isset($message[$row['md5sum']])) unset($result[$key]);
                                         else $message[$row['md5sum']] = $row['node'];
                                    }
                               }	

                               $results = array_merge($results,$result);
                         }
                   }
             }

             /* Sort it if we have more than 1 location*/
             //if(count($location) > 1) 
             usort($results, create_function('$a, $b', 'return $a["micro_ts"] > $b["micro_ts"] ? 1 : -1;'));

             foreach($results as $val) 
             {

                  $row = (object) $val;        
                  $data = preg_replace('!\\\015\\\012!',"\r\n",$row->msg);
                  $size->data=strlen($data);
                  $pkt = '';

                  if($text) 
                  {	   
                       $sec = intval($row->micro_ts / 1000000);
                       $usec = $row->micro_ts - ($sec*1000000);
                       $buf .= "U ".date("Y/m/d H:i:s", $sec).".".$usec." "
                          .$row->source_ip.":".$row->source_port." -> "
                          .$row->destination_ip.":".$row->destination_port."\r\n";
                       $buf.=$data."\r\n";             
                  } else {
                       //Ethernet + IP + UDP
                       $size->total=$size->ethernet + $size->ip + $size->udp;
                       //+Data
                       $size->total+=$size->data;
                       //Pcap record
                       $pcaprec_hdr = new pcaprec_hdr();
                       $pcaprec_hdr->ts_sec = intval($row->micro_ts / 1000000);  //4
                       $pcaprec_hdr->ts_usec = $row->micro_ts - ($pcaprec_hdr->ts_sec*1000000); //4   
                       $pcaprec_hdr->incl_len = $size->total; //4
                       $pcaprec_hdr->orig_len = $size->total; //4
                       $pcaprec_packet = pack("llll", $pcaprec_hdr->ts_sec, $pcaprec_hdr->ts_usec, 
                       $pcaprec_hdr->incl_len, $pcaprec_hdr->orig_len);
                       $buf.=$pcaprec_packet;

                       //ethernet header
                       $buf.=$ethernet;

                       //UDP
                       $udp_hdr = new udp_header();
                       $udp_hdr->src_port = $row->source_port;
                       $udp_hdr->dst_port = $row->destination_port;
                       $udp_hdr->length = $size->udp + $size->data; 
                       $udp_hdr->checksum = 0;

                       //Calculate UDP checksum
                       $pseudo = pack("nnnna*", $udp_hdr->src_port,$udp_hdr->dst_port, $udp_hdr->length, $udp_hdr->checksum, $data);
                       $udp_hdr->checksum = $this->checksum($pseudo);

                       //IPHEADER

                       $ipv4_hdr = new ipv4_packet();
                       $ip_ver = 4;
                       $ip_len = 5;
                       $ip_frag_flag = "010";
                       $ip_frag_oset = "0000000000000";
                       $ipv4_hdr->ver_len = $ip_ver . $ip_len;
                       $ipv4_hdr->tos = "00";
                       $ipv4_hdr->total_len = $size->ip + $size->udp + $size->data;;
                       $ipv4_hdr->ident = 19245;
                       $ipv4_hdr->fl_fr = 4000;
                       $ipv4_hdr->ttl = 30;
                       $ipv4_hdr->proto = 17;
                       $ipv4_hdr->checksum = 0;
                       $ipv4_hdr->src_ip = ip2long($row->source_ip);
                       $ipv4_hdr->dst_ip = ip2long($row->destination_ip);
                       $pseudo = pack('H2H2nnH4C2nNN', $ipv4_hdr->ver_len,$ipv4_hdr->tos,$ipv4_hdr->total_len, $ipv4_hdr->ident,
                       $ipv4_hdr->fl_fr, $ipv4_hdr->ttl,$ipv4_hdr->proto,$ipv4_hdr->checksum, $ipv4_hdr->src_ip, $ipv4_hdr->dst_ip);
                       $ipv4_hdr->checksum = $this->checksum($pseudo);

                       $pkt = pack('H2H2nnH4C2nNNnnnna*', $ipv4_hdr->ver_len,$ipv4_hdr->tos,$ipv4_hdr->total_len, $ipv4_hdr->ident,
                            $ipv4_hdr->fl_fr, $ipv4_hdr->ttl,$ipv4_hdr->proto,$ipv4_hdr->checksum, $ipv4_hdr->src_ip, $ipv4_hdr->dst_ip,
                            $udp_hdr->src_port,$udp_hdr->dst_port, $udp_hdr->length, $udp_hdr->checksum, $data);
                  }

                 //IP/UDP and DATA header	
                 $buf.=$pkt;
             }

             $pcapfile="HOMER_$fileid";
             $pcapfile .= $text ? ".txt" : ".pcap";

             // Check if local PCAP or CSHARK enabled
             if (CSHARK == 1 && !$text) 
             {

                  $apishark = CSHARK_URI."/api/v1/".CSHARK_API."/upload";
                  $pfile = PCAPDIR."/".$pcapfile;
                  $fileHandle = fopen($pfile, 'w') or die("Error opening file");
                  fwrite($fileHandle, $buf);
                  fclose($fileHandle); 

                  $ch = curl_init();
                  curl_setopt($ch, CURLOPT_HEADER, 0);
                  curl_setopt($ch, CURLOPT_VERBOSE, 0);
                  curl_setopt($ch, CURLOPT_RETURNTRANSFER, true);
                  curl_setopt($ch, CURLOPT_USERAGENT, "Mozilla/4.0 (compatible;)");
                  curl_setopt($ch, CURLOPT_URL, $apishark);
                  curl_setopt($ch, CURLOPT_POST, true);
                  // curl_setopt($ch, CURLOPT_SSL_VERIFYPEER, true);
                  // curl_setopt($ch, CURLOPT_SSL_VERIFYHOST, 2);
                  // curl_setopt($ch, CURLOPT_CAINFO, getcwd() . "/opt/ca-cert-cshark.crt");

                  $post = array(
                       "file"=>"@$pfile",
                  );

                  curl_setopt($ch, CURLOPT_POSTFIELDS, $post);
                  $response = curl_exec($ch);

                  $json=json_decode($response,true);
                  $url = CSHARK_URI."/captures/".$json[id];
                  header('Location: '.$url);
                  // Remove Temp
                  unlink($pfile);
                  exit;
                  
             } else {
                 return array($pcapfile, strlen($buf), $buf);
             } 

       }
}

?>
