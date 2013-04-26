<?php
/*
 * HOMER Web Interface
 * Homer's SIP DataTable Class
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


class SipDataTable extends DataTable_DataTable
{
  /**
   * @param DataTable_Config $config
   */
  public function __construct(DataTable_Config $config = null)
  {
    // create first column

    $column0 = new DataTable_Column();
    $column0->setName("checkbox")
            ->setTitle("&#916;")
            ->setGetMethod("getCheckbox")
            ->setSortKey("id")
            ->setIsSortable(false);
    
    $column1 = new DataTable_Column();
    $column1->setName("id")
            ->setTitle("id")
            ->setGetMethod("getID")
            ->setSortKey("id")
            ->setIsSortable(true)
            ->setIsDefaultSort(true);


    // create second column
    $column2 = new DataTable_Column();
    $column2->setName("date")
            ->setTitle("date")
            ->setGetMethod("getDate")
            ->setSortKey("date")
            ->setIsSortable(true)
            ->setIsSearchable(false);
            
    // create second column
    $column3 = new DataTable_Column();
    $column3->setName("micro_ts")
            ->setTitle("ts")
            ->setGetMethod("getMicroTs")
            ->setSortKey("micro_ts")
            ->setIsSortable(true)
            ->setIsVisible(false)                        
            ->setIsSearchable(false);            

    // create second column
    $column4 = new DataTable_Column();
    $column4->setName("method")
            ->setTitle("method")
            ->setGetMethod("getMethod")
            ->setSortKey("method")
            ->setIsSortable(true)
            ->setIsSearchable(false);            
            
    $column5 = new DataTable_Column();
    $column5->setName("reply_reason")
            ->setTitle("Reason")
            ->setGetMethod("getReplyReason")
            ->setSortKey("reply_reason")
            ->setIsVisible(false)                        
            ->setIsSortable(true)
            ->setIsSearchable(false);            

    // create second column
    $column6 = new DataTable_Column();
    $column6->setName("ruri")
            ->setTitle("ruri")
            ->setGetMethod("getRuri")
            ->setSortKey("ruri")
            ->setIsSortable(true)
            ->setIsVisible(false)                        
            ->setIsSearchable(false);            

    $column7 = new DataTable_Column();
    $column7->setName("ruri_user")
            ->setTitle("Ruser")
            ->setGetMethod("getRuriUser")
            ->setSortKey("ruri_user")
            ->setIsSortable(true)
            ->setIsSearchable(true);            


    $column8 = new DataTable_Column();
    $column8->setName("from_user")
            ->setTitle("from user")
            ->setGetMethod("getFromUser")
            ->setSortKey("from_user")
            ->setIsSortable(true)
            ->setIsSearchable(true);            
            
    $column9 = new DataTable_Column();
    $column9->setName("from_tag")
            ->setTitle("from tag")
            ->setGetMethod("getFromTag")
            ->setSortKey("from_tag")
            ->setIsSortable(true)
            ->setIsVisible(false)                        
            ->setIsSearchable(false);                        

    $column10 = new DataTable_Column();
    $column10->setName("to_user")
            ->setTitle("to user")
            ->setGetMethod("getToUser")
            ->setSortKey("to_user")
            ->setIsSortable(true)
            ->setIsSearchable(true);                        

    $column11 = new DataTable_Column();
    $column11->setName("to_tag")
            ->setTitle("to tag")
            ->setGetMethod("getToTag")
            ->setSortKey("to_tag")
            ->setIsSortable(true)
            ->setIsVisible(false)            
            ->setIsSearchable(false);                        

    $column12 = new DataTable_Column();
    $column12->setName("pid_user")
            ->setTitle("pid")
            ->setGetMethod("getPidUser")
            ->setSortKey("pid_user")
            ->setIsSortable(true)
            ->setIsSearchable(true);                        

    $column13 = new DataTable_Column();
    $column13->setName("contact_user")
            ->setTitle("Cont. user")
            ->setGetMethod("getContactUser")
            ->setSortKey("contact_user")
            ->setIsSortable(true)
            ->setIsVisible(false)            
            ->setIsSearchable(false);                        

    $column14 = new DataTable_Column();
    $column14->setName("auth_user")
            ->setTitle("Auth user")
            ->setGetMethod("getAuthUser")
            ->setSortKey("auth_user")
            ->setIsSortable(true)
            ->setIsVisible(false)            
            ->setIsSearchable(true);                        

    $column15 = new DataTable_Column();
    $column15->setName("callid")
            ->setTitle("Call-id")
            ->setGetMethod("getCallId")
            ->setSortKey("callid")
            ->setIsSortable(true)
            ->setIsSearchable(true);                        

    $column16 = new DataTable_Column();
    $column16->setName("callid_aleg")
            ->setTitle("Call-ID Aleg")
            ->setGetMethod("getCalIdAleg")
            ->setSortKey("callid_aleg")
            ->setIsSortable(true)
            ->setIsVisible(false)            
            ->setIsSearchable(false);                        

    $column17 = new DataTable_Column();
    $column17->setName("via_1")
            ->setTitle("Via")
            ->setGetMethod("getVia1")
            ->setSortKey("via_1")
            ->setIsSortable(true)
            ->setIsVisible(false)
            ->setIsSearchable(false);                        

    $column18 = new DataTable_Column();
    $column18->setName("via_1_branch")
            ->setTitle("Via Branch")
            ->setGetMethod("getVia1Branch")
            ->setSortKey("via_1_branch")
            ->setIsSortable(true)
            ->setIsVisible(false)
            ->setIsSearchable(false);                        

    $column19 = new DataTable_Column();
    $column19->setName("cseq")
            ->setTitle("Cseq")
            ->setGetMethod("getCseq")
            ->setSortKey("cseq")
            ->setIsSortable(true)
            ->setIsVisible(false)
            ->setIsSearchable(false);                        

    $column20 = new DataTable_Column();
    $column20->setName("diversion")
            ->setTitle("Diversion")
            ->setGetMethod("getDiversion")
            ->setSortKey("diversion")
            ->setIsSortable(true)
            ->setIsVisible(false)
            ->setIsSearchable(false);                        

    $column21 = new DataTable_Column();
    $column21->setName("reason")
            ->setTitle("Reason")
            ->setGetMethod("getReason")
            ->setSortKey("reason")
            ->setIsVisible(false)
            ->setIsSortable(true)
            ->setIsSearchable(false);                        

    $column22 = new DataTable_Column();
    $column22->setName("content_type")
            ->setTitle("Cont-Type")
            ->setGetMethod("getContentType")
            ->setSortKey("content_type")
            ->setIsSortable(true)
            ->setIsVisible(false)
            ->setIsSearchable(false);                        

    $column23 = new DataTable_Column();
    $column23->setName("authorization")
            ->setTitle("Authoriz.")
            ->setGetMethod("getAuthorization")
            ->setSortKey("authorization")
            ->setIsVisible(false)
            ->setIsSortable(true)
            ->setIsSearchable(false);   

    /* SCHEMA */
    if(SQL_SCHEMA_VERSION == 2) {
    
        $column23->setName("auth")->setSortKey("auth");        
    }

    $column24 = new DataTable_Column();
    $column24->setName("user_agent")
            ->setTitle("UAG")
            ->setGetMethod("getUserAgent")
            ->setIsVisible(false)
            ->setSortKey("user_agent")
            ->setIsSortable(true)
            ->setIsSearchable(false);                        

    $column25 = new DataTable_Column();
    $column25->setName("source_ip")
            ->setTitle("Src.IP")
            ->setGetMethod("getSourceIp")
            ->setSortKey("source_ip")
            ->setIsSortable(true)
            ->setIsSearchable(false);
            
    $column26 = new DataTable_Column();
    $column26->setName("source_port")
            ->setTitle("Src.Port")
            ->setGetMethod("getSourcePort")
            ->setSortKey("source_port")
            ->setIsSortable(true)
            ->setIsSearchable(false);            

    $column27 = new DataTable_Column();
    $column27->setName("destination_ip")
            ->setTitle("Dest.IP")
            ->setGetMethod("getDestinationIp")
            ->setSortKey("destination_ip")
            ->setIsSortable(true)
            ->setIsSearchable(false);
            
    $column28 = new DataTable_Column();
    $column28->setName("destination_port")
            ->setTitle("Dst. Port")
            ->setGetMethod("getDestinationPort")
            ->setSortKey("destination_port")
            ->setIsSortable(true)
            ->setIsSearchable(false);            

    $column29 = new DataTable_Column();
    $column29->setName("contact_ip")
            ->setTitle("Contact IP")
            ->setGetMethod("getContactIp")
            ->setSortKey("contact_ip")
            ->setIsVisible(false)
            ->setIsSortable(true)
            ->setIsSearchable(false);
            
    $column30 = new DataTable_Column();
    $column30->setName("contact_port")
            ->setTitle("Contact Port")
            ->setGetMethod("getContactPort")
            ->setSortKey("contact_port")
            ->setIsSortable(true)
            ->setIsVisible(false)
            ->setIsSearchable(false);            

    $column31 = new DataTable_Column();
    $column31->setName("originator_ip")
            ->setTitle("Originator.IP")
            ->setGetMethod("getOriginatorIp")
            ->setSortKey("originator_ip")
            ->setIsVisible(false)
            ->setIsSortable(true)
            ->setIsSearchable(false);
            
    $column32 = new DataTable_Column();
    $column32->setName("originator_port")
            ->setTitle("Orig. Port")
            ->setGetMethod("getOriginatorPort")
            ->setSortKey("originator_port")
            ->setIsVisible(false)
            ->setIsSortable(true)
            ->setIsSearchable(false);            

    $column33 = new DataTable_Column();
    $column33->setName("family")
            ->setTitle("Family")
            ->setGetMethod("getFamily")
            ->setSortKey("family")
            ->setIsVisible(false)
            ->setIsSortable(true)
            ->setIsDefaultSort(false);

    // create second column
    $column34 = new DataTable_Column();
    $column34->setName("proto")
            ->setTitle("Proto")
            ->setGetMethod("getProto")
            ->setSortKey("proto")
            ->setIsVisible(false)
            ->setIsSortable(true)
            ->setIsSearchable(false);            

    // create second column
    $column35 = new DataTable_Column();
    $column35->setName("type")
            ->setTitle("TYPE")
            ->setGetMethod("getType")
            ->setSortKey("type")
            ->setIsVisible(false)
            ->setIsSortable(true)
            ->setIsSearchable(false);            

    // create third column
    $column36 = new DataTable_Column();
    $column36->setName("node")
            ->setTitle("Node")
            ->setGetMethod("getNode")
            ->setSortKey("node")
            ->setIsSortable(true)
            ->setIsSearchable(false)                        
            ->setIsDefaultSort(false);
    
    // create the actions column
    $column37 = new DataTable_Column();
    $column37->setName("actions")
            ->setTitle("Actions")
            ->setGetMethod("getActions");
    
    // create an invisible column
    $column38 = new DataTable_Column();
    $column38->setName("invisible")
            ->setTitle("Invisible")
            ->setIsVisible(false)
            ->setGetMethod("getInvisible");

    // create third column
    $column39 = new DataTable_Column();
    $column39->setName("loctable")
            ->setTitle("loctable")
            ->setGetMethod("getLoctable")
            ->setSortKey("loctable")
            ->setIsSortable(false)
            ->setIsSearchable(false)                        
            ->setIsVisible(false)                        
            ->setIsDefaultSort(false);

    // create third column
    $column40 = new DataTable_Column();
    $column40->setName("tnode")
            ->setTitle("tnode")
            ->setGetMethod("getTnode")
            ->setSortKey("tnode")
            ->setIsSortable(false)
            ->setIsSearchable(false)                        
            ->setIsVisible(false)                        
            ->setIsDefaultSort(false);

    $column41 = new DataTable_Column();
    $column41->setName("callidtag")
            ->setTitle("callidtag")
            ->setGetMethod("getCallIdTag")
            ->setSortKey("callid")
            ->setIsSortable(false)
            ->setIsSearchable(false)                        
            ->setIsVisible(false)                        
            ->setIsDefaultSort(false);            

    
    // create config
    $config = new DataTable_Config();
    
    // add columns to collection
    $config->getColumns()->add($column0)
                         ->add($column1)
                         ->add($column2)
                         ->add($column3)
                         ->add($column4)
                         ->add($column5)
                         ->add($column6)
                         ->add($column7)
                         ->add($column8)
                         ->add($column9)
                         ->add($column10)
                         ->add($column11)
                         ->add($column12)
                         ->add($column13)
                         ->add($column14)
                         ->add($column15)
                         ->add($column16)
                         ->add($column17)
                         ->add($column18)
                         ->add($column19)
                         ->add($column20)
                         ->add($column21)
                         ->add($column22) 
                         ->add($column23)
                         ->add($column24)
                         ->add($column25)
                         ->add($column26)
                         ->add($column27)
                         ->add($column28)
                         ->add($column29)
                         ->add($column30)
                         ->add($column31)
                         ->add($column32)
                         ->add($column33)
                         ->add($column34)
                         ->add($column35)
                         ->add($column36)
//                         ->add($column37)
                         ->add($column38)
                         ->add($column39)
                         ->add($column40)
                         ->add($column41);
     
    // build the language configuration
    $languageConfig = new DataTable_LanguageConfig();
    $languageConfig->setPaginateFirst("Beginning")
                   ->setPaginateLast("End")
                   ->setSearch("Find it:");

    // add LangugateConfig to the DataTableConfig object
    $config->setLanguageConfig($languageConfig);

    $dom = "<\"top\"fip<\"clear\">>rt<\"bottom\"lip<\"clear\">>";
    $config->setDom($dom);

    // set data table options
    $config->setClass("display")
           ->setDisplayLength(25)
           ->setIsPaginationEnabled(true)
           ->setIsLengthChangeEnabled(true)
           ->setIsFilterEnabled(true)
           ->setIsInfoEnabled(true)
           ->setIsSortEnabled(true)
           ->setIsAutoWidthEnabled(false)
           ->setIsScrollCollapseEnabled(false)
           ->setPaginationType(DataTable_Config::PAGINATION_TYPE_FULL_NUMBERS)
           ->setIsJQueryUIEnabled(true)
           ->setIsServerSideEnabled(true);

    // pass DataTable_Config to the parent
    parent::__construct($config);
  }
  
  /* return our columns */

  public function getColumns()
  {
       return $this->config->getColumns();
  }

  /**
   * Set the ISipService implementation
   * 
   * This is the service object where we will pull our data from
   * 
   * @param ISipService $browserService
   */
  public function setBrowserService(ISipService $browserService)
  {
    $this->browserService = $browserService;
  }

  /**
   * Load the data for a request
   * 
   * This demo emulates loading data from a database and performing
   * a count of the total results, limiting them, ordering them, and
   * searching if a search term is passed in.
   * 
   * @see DataTable_DataTable::loadData()
   */
  public function loadData(DataTable_Request $request)
  {
    // get the name of the sort property that was passed in
    $sortProperty = $this->config->getColumns()->get($request->getSortColumnIndex())->getSortKey();

     //getHomer

    // check if a search term was passed in
    if($request->hasSearch() || $request->hasColumnSearch()){

    	// get the total number of results (for pagination)
    	$totalLength = $this->browserService->searchAll($request->getSearch(),
    			$this->getSearchableColumnNames(),
    			$request->getDisplayStart(),
    			$request->getDisplayLength(),
    			$sortProperty,
    			$request->getSortDirection(),
    			true,
    			$request->getHomer(),
    			$request->getSearchtColumns(),
    			$this
    	);
      
      // call the searchAll() service method
      $results = $this->browserService->searchAll($request->getSearch(), 
                                                  $this->getSearchableColumnNames(), 
                                                  $request->getDisplayStart(), 
                                                  $request->getDisplayLength(), 
                                                  $sortProperty, 
                                                  $request->getSortDirection(),
                                                  false,
                                                  $request->getHomer(),
                                                  $request->getSearchtColumns(),
                                                  $this
                                                  );

    
    } else {
    	
    	// get the total number of results (for pagination)
    	$totalLength = $this->browserService->getAll($request->getDisplayStart(),
    			$request->getDisplayLength(),
    			$sortProperty,
    			$request->getSortDirection(),
    			true,
    			$request->getHomer());
      
      // call the getAll() service method
      $results = $this->browserService->getAll($request->getDisplayStart(), 
                                               $request->getDisplayLength(), 
                                               $sortProperty, 
                                               $request->getSortDirection(),
                                               false,
                                               $request->getHomer());
      
    }

    // return the final result set
    return new DataTable_DataResult($results, $totalLength, $totalLength);
  }

  /**
   * (non-PHPdoc)
   * @see DataTable_DataTable::getTableId()
   */
  public function getTableId()
  {
    return 'SIPTable';
  }


  /*data from homer*/
  public function getSearchData() 
  {
        $search = $this->searchRequest;

        /* check */
        foreach($search as $key=>$value) {
              if($value == "" ||$value == '0' || $value == " " || is_null($value)) {              
                  unset($search[$key]);
              }                
        }  

        $json = json_encode($search);
        //$json = preg_replace('/"(-?\d+\.?\d*)"/', '$1', $json);        
        
        $html = "aoData.push( { 'name': 'homersearch', 'value': '".$json."' } );\n";              
        $html .= "aoData.push( { 'name': 'close', 'value': '1' } );\n";              
        return $html;
  }


  /**
   * Format the data for the 'Actions' column
   * 
   * @param Browser $browser
   */
  protected function getActions(SipResult $sipresult)
  {
    $html = "<a href=\"#\" onclick=\"alert('Viewing: {$sipresult->getId()}');\">View</a>";
    $html .= ' | ';
    $html .= "<a href=\"#\" onclick=\"confirm('Delete {$sipresult->getId()}?');\">Delete</a>";
    return $html;
  }
  
  /**
   * Format the data for the 'invisible' column
   * 
   * @param Browser $browser
   */
  protected function getInvisible(SipResult $sipresult)
  {
    return 'invisible content: ' . $sipresult->getId();
  }

  /**
   * Add a callback function for 'fnRowCallback'
   * 
   * @return string
   */
  protected function getRowCallbackFunction()
  {
    return "
            function( nRow, aData, iDisplayIndex, iDisplayIndexFull ) {

    			/* Bold the grade for all 'A' grade browsers */
    			if ( aData[{$this->getColumnIndexByName('id')}] == 'A' )
    			{
    				$('td:eq({$this->getColumnIndexByName('id')})', nRow).html( '<b>A</b>' );
    			}
                        
			/* Colour by CALLID */    	
                        var callid = aData[{$this->getColumnIndexByName('callid')}];
			/* Colour by CALLID + TAG */    	
                        // var callid = aData[{$this->getColumnIndexByName('callidtag')}];

    			if(initColors[callid] === undefined) {
                            var r = Math.floor(Math.random()*256+100);
                            var g = Math.floor(Math.random()*256+100);
                            var b = Math.floor(Math.random()*256+100);
                            initColors[callid]='rgb('+r+','+g+','+b+')';                            
    			} 
                        $(nRow).css('background-color',initColors[callid]);
    			return nRow;
            }
    ";
  }

// border-top: 0; border-bottom: 1px solid #f5f5f5; border-left: 1px solid #f5f5f5; border-right: 1px solid #f5f5f5;
  
  protected function getServerDataFunction()
  {
    return "
            function( sSource, aoData, fnCallback ) {
                    {$this->getSearchData()}                    
                    var sendData = 'action=search&data=' + $.toJSON(aoData);
                    $.ajax( {
                            'dataType': 'json',
                            'type': '".AJAXTYPE."',
                            'url': sSource,
                            'data': sendData,
                            'timeout': '".AJAXTIMEOUT."',
                            'error': function(){ alert(\"Timeout error. Please take small timeintervall\") },
                            'success': fnCallback
                    } );
            }
    ";
  }
}
