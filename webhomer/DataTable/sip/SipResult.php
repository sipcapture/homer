<?php
/*
 * HOMER Web Interface
 * Homer's SipResult class
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

class SipResult
{
  private $id;
  private $date;
  private $micro_ts;
  private $method;
  private $reply_reason;
  private $ruri;
  private $ruri_user;
  private $from_user;
  private $from_tag;
  private $to_user;
  private $to_tag;
  private $pid_user;
  private $contact_user;
  private $auth_user;
  private $callid;
  private $callid_aleg;
  private $via_1;
  private $via_1_branch;
  private $cseq;
  private $diversion;
  private $reason;
  private $content_type;
  private $authorization;
  private $user_agent;
  private $source_ip;
  private $source_port;
  private $destination_ip;
  private $destination_port;
  private $contact_ip;
  private $contact_port;
  private $originator_ip;
  private $originator_port;
  private $proto;
  private $family;
  private $type;
  private $node;  
  private $callidtag;  
  private $rtp_stat;  

  private $loctable;
  private $tnode;
  private $location;
  private $tablename;
  private $unique = 0;
  

  public function __construct($data, $location)
  {

	foreach($data as $key=>$value) {
	      if(property_exists('SipResult',$key)) $this->$key = $value;
	}
		
	$this->location = $location;
  }

  public function getID()
  {
      return $this->id;
  }
  
  public function getCheckbox()
  {
      return "<input style='input' type='checkbox' id='cb".$this->id."' name='cid[]' value='". substr($this->micro_ts,0,-2)."' onclick='checkboxEvent(this.value, this.id);' />";
  }

  public function getDate()
  {
      //Corellation for 3000 nanoseconds
      //if($row->tnode == 2) (int) $row->micro_ts - (int) 4000;
      $seconds = intval($this->micro_ts / 1000000);  //4
      $ms = $this->micro_ts - ($seconds*1000000); //4      
      //return date("H:i:s", strtotime($this->date));  
      if(!defined('FORMAT_DATE_RESULT')) $format_date = "H:i:s";
      else $format_date = FORMAT_DATE_RESULT;
      return date($format_date, $seconds).".".$ms;
      //return $this->date;
  }
  
  public function getMicroTs()
  {
     return $this->micro_ts;
     //return substr($this->micro_ts, -9);
  }
  
   public function getMethod()
  {

      if(!defined('MESSAGE_POPUP')) $popuptype = 1;
      else $popuptype = MESSAGE_POPUP;

      $fd = date("Y-m-d", strtotime($this->date));
      $ft = date("H:i:s", strtotime($this->date));

      $url = "utils.php?task=sipmessage&id=".$this->id."&popuptype=".$popuptype;
      $url .= "&from_time=".$ft."&from_date=".$fd."&tnode=".$this->tnode;
      $url .= "&tablename=".$this->tablename;
      
      $rtpinfo = "";
      if(preg_match('/=/',$this->rtp_stat)) $rtpinfo = " <b>(R)</b>";     
      return "<a href=\"javascript:popMessage2(".$popuptype.",'".$this->id."','".$url."');\">".$this->method."</a>".$rtpinfo;
  }

  public function getReplyReason()
  {
      return $this->reply_reason;
  }
  
  public function getRuri()
  {
      return $this->ruri;
  }
  
  public function getRuriUser()
  {
      return $this->ruri_user;
  }
  
  public function getFromUser()
  {
      return $this->from_user;
  }  
  
  public function getFromTag()
  {
      return $this->from_tag;
  }  

  public function getToUser()
  {
      return $this->to_user;
  }
  
  public function getToTag()
  {
      return $this->to_tag;
  }
  
  public function getPidUser()
  {
      return $this->pid_user;
  }
 
  public function getContactUser()
  {
      return $this->contact_user;
  }

  public function getAuthUser()
  {
      return $this->auth_user;
  }

  public function getCallId()
  {

   $search = json_decode($_SESSION['homersearch']);
   if(!defined('CFLOW_POPUP')) $popuptype = 1;
   else $popuptype = CFLOW_POPUP;
      
   $fd = date("Y-m-d", strtotime($search->from_date));
   $td = date("Y-m-d", strtotime($search->to_date));   
   $url = "cflow.php?cid[]=".$this->callid;
   $url .= "&from_time=".$search->from_time."&to_time=".$search->to_time."&from_date=".$fd."&to_date=".$td;
   $url .= "&callid_aleg=".$search->b2b."&popuptype=".$popuptype."&unique=".$search->unique."&location[]=".implode("&location[]=", $search->location);

   return "<a alt='callflow' href=\"javascript:showCallFlow2($popuptype, '".$this->callid."','".$url."');\">".$this->callid."</a>";

  }  
  
  public function getCallIdTag()
  {
  
      return $this->callid.$this->from_tag;
      //return $this->callid;
  }  
  
  public function getCalIdAleg()
  {
      return $this->callid_aleg;
  }  

  public function getVia1()
  {
      return $this->via_1;
  }
  
  public function getVia1Branch()
  {
      return $this->via_1_branch;
  }
  
  public function getCseq()
  {
      return $this->cseq;
  }
 
  public function getDiversion()
  {
      return $this->diversion;
  }

  public function getReason()
  {
      return $this->reason;
  }

  public function getContentType()
  {
      return $this->content_type;
  }
 
  public function getAuthorization()
  {
      return $this->authorization;
  }

  public function getUserAgent()
  {
      return $this->user_agent;
  }

  public function getSourceIP()
  {
       if (GEOIP_LINK==1) {
                return "<a href='".GEOIP_URL.$this->source_ip."' target='_blank'>".$this->source_ip."</a>";
       } else if (GEOIP_LINK==2) {
                $url = "http://api.hostip.info/get_html.php?ip=".$this->source_ip;
       return "<a href=\"javascript:popAny('".$url."', '".$this->source_ip."');\">".$this->source_ip."</a>";
       } else {
                return $this->source_ip;
       }
  }

  public function getSourcePort()
  {
      return $this->source_port;
  }  

  public function getDestinationIP()
  {
       if (GEOIP_LINK==1) {
                return "<a href='".GEOIP_URL.$this->destination_ip."' target='_blank'>".$this->destination_ip."</a>";
       } else if (GEOIP_LINK==2) {
                $url = "http://api.hostip.info/get_html.php?ip=".$this->destination_ip;
       return "<a href=\"javascript:popAny('".$url."', '".$this->destination_ip."');\">".$this->destination_ip."</a>";
       } else {
                return $this->destination_ip;
       }

  }
  
  public function getDestinationPort()
  {
      return $this->destination_port;
  }
  
  public function getContactIp()
  {
      return $this->contact_ip;
  }
  
  public function getContactPort()
  {
      return $this->contact_port;
  }
 
  public function getOriginatorIp()
  {
      return $this->originator_ip;
  }

  public function getOriginatorPort()
  {
      return $this->originator_port;
  }

  public function getProto()
  {
      return $this->proto;
  }
 
  public function getFamily()
  {
      return $this->family;
  }

  public function getType()
  {
      return $this->type;
  }
  
  public function getNode()
  {
      return $this->node;
  }
  
  public function getLoctable()
  {
      return $this->loctable;
  }
  
  public function getTnode()
  {
      return $this->tnode;
  }

}
