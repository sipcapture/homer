<?php

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
//  private $contenttype;
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

  private $loctable;
  private $tnode;
  private $location;
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
      return "<input style='input' type='checkbox' id='cb".$this->id."' name='cid[]' value='". substr($this->micro_ts,0,-2)."' onclick='calculateDelta(this.value);' />";
  }

  public function getDate()
  {
      //Corellation for 3000 nanoseconds
      //if($row->tnode == 2) (int) $row->micro_ts - (int) 4000;
      $seconds = $this->micro_ts / 1000000;                                                                                      
      $ms = $this->micro_ts % 1000000;                                                                                      
      //return date("H:i:s", strtotime($this->date));  
      return date("H:i:s", $seconds).".".$ms;
      //return $this->date;
  }
  
  public function getMicroTs()
  {
     return $this->micro_ts;
     //return substr($this->micro_ts, -9);
  }
  
  public function getMethod()
  {
      return "<a href=\"javascript:showMessage('".$this->id."','".$this->loctable."','".$this->tnode."','". implode(",",$this->location)."');\">".$this->method."</a>";
      //return $this->method;
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
  
      return "<a alt='callflow' href=\"javascript:showCallFlow('".$this->id."','".$this->loctable."','".$this->tnode."','".
              implode(',',$this->location)."','".$this->unique."', 1 );\">".$this->callid."</a>";
      //return $this->callid;
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
      return $this->source_ip;
  }

  public function getSourcePort()
  {
      return $this->source_port;
  }  

  public function getDestinationIP()
  {
      return $this->destination_ip;
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