<?php

/**
 * This file is part of the DataTable package
 * 
 * (c) Marc Roulias <marc@lampjunkie.com>
 * 
 * For the full copyright and license information, please view the LICENSE
 * file that was distributed with this source code.
 */

/**
 * This is the base class that all DataTables need to extend
 * 
 */
abstract class DataTable_DataTable
{
  /**
   * The DataTable_Config object
   * @var DataTable_Config
   */
  protected $config;

  
  protected $searchRequest;
  
  /**
   * The Ajax source url
   * 
   * @var string
   */
  protected $ajaxDataUrl;
  
  /**
   * The server parameters passed in the AJAX request
   * 
   * @var DataTable_Request
   */
  protected $request;

  /**
   * Array to store javascript callback functions each
   * with a unique key
   * 
   * @var array
   */
  protected $jsonFunctions;
  
  /**
   * Array to store mapping of column name => position index
   *
   * @var array
   */
  protected $columnIndexNameCache;

  /**
   * Creates a new DataTable using the given DataTable_Config object
   * 
   * @param DataTable_Config $config
   */
  public function __construct(DataTable_Config $config = null)
  {
    if(is_null($config)){
      throw new DataTable_DataTableException("A DataTable_Config object is required.");
    }

    $this->config = $config;
  }

  /**
   * Get a unique id for the current DataTable
   * 
   * This value is used as the HTML id on the table when it
   * is rendered in the HTML ouput
   * 
   * @return string
   */
  abstract public function getTableId();
  
  /**
   * Load data for an AJAX request
   * 
   * This method must return a DataTable_DataResult object
   * 
   * @param DataTable_ServerParameterHolder $parameters
   * @return DataTable_DataResult
   */
  abstract protected function loadData(DataTable_Request $request);

  /**
   * Override this method to return the javascript function that
   * will be passed as the 'fnRowCallback' option
   * 
   * @return string
   */
  protected function getRowCallbackFunction(){}
  
    /**
   * Override this method to return the javascript function that
   * will be passed as the 'fnRowCallback' option
   * 
   * @return string
   */
  protected function getServerDataFunction(){}

  /**
   * Override this method to return the javascript function that
   * will be passed as the 'fnInitComplete' option
   * 
   * @return string
   */
  protected function getInitCompleteFunction(){}
  
  /**
   * Override this method to return the javascript function that
   * will be passed as the 'fnDrawCallback' option
   * 
   * @return string
   */
  protected function getDrawCallbackFunction(){}
  
  /**
   * Override this method to return the javascript function that
   * will be passed as the 'fnFooterCallback' option
   * 
   * @return string
   */
  protected function getFooterCallbackFunction(){}
  
  /**
   * Override this method to return the javascript function that
   * will be passed as the 'fnHeaderCallback' option
   * 
   * @return string
   */
  protected function getHeaderCallbackFunction(){}
  
  /**
   * Override this method to return the javascript function that
   * will be passed as the 'fnInfoCallback' option
   * 
   * @return string
   */
  protected function getInfoCallbackFunction(){}
  
  /**
   * Override this method to return the javascript function that
   * will be passed as the 'fnCookieCallback' option
   * 
   * @return string
   */
  protected function getCookieCallbackFunction(){}
  
  /**
   * Render the initial HTML and javascript to instantiate and display the DataTable
   * 
   * @return string
   */
  public function render()
  {
    if(is_null($this->config)){
      throw new DataTable_DataTableException("A DataTable_Config object is required.");
    }

    return $this->renderHtml() . $this->renderJs();
  }
  
  /**
   * Get the JSON formatted date for a AJAX request
   * 
   * @param DataTable_ServerParameterHolder $serverParameters
   * @return string
   */
  public function renderJson(DataTable_Request $request)
  {
    if(is_null($this->config)){
      throw new DataTable_DataTableException("A DataTable_Config object is required.");
    }

    $this->request = $request;
    $dataTableDataResult = $this->loadData($request);
    return $this->renderReturnData($dataTableDataResult);
  }

  /**
   * Render the return JSON data for the AJAX request with the DataTable_DataResult
   * returned from the current DataTable's loadData() method
   * 
   * @param DataTable_DataResult $result
   */
  protected function renderReturnData(DataTable_DataResult $result)
  {
    $rows = array();

    $mydata = $result->getData();

    if(count($mydata)) {
        foreach($mydata as $object){

          $row = array();
          foreach($this->config->getColumns() as $column){
            $row[] = $this->getDataForColumn($object, $column);
          }

          $rows[] = $row;
        }
    }

    $data = array(
                        'iTotalRecords' => $result->getNumTotalResults(),
                        'iTotalDisplayRecords' => !is_null($result->getNumFilteredResults()) ?
                                        $result->getNumFilteredResults() : $result->getNumTotalResults(),
                        'aaData' => $rows,
                        'sEcho' => $this->request->getEcho(),
    );

    return json_encode($data);
  }


  /**
   * Get the data for for a column from the given data object row
   * 
   * This method will first try calling the get method on the current
   * DataTable object. If the method doesn't exist, then it will default
   * to calling the method on the object for the current row
   * 
   * @param object $object
   * @param DataTable_Column $column
   * @return mixed
   */
  protected function getDataForColumn($object, DataTable_Column $column)
  {
    $getter = $column->getGetMethod();

    if(method_exists($this, $getter)){
      return call_user_func(array($this, $getter), $object);  
    } else {
    
      if(method_exists($object, $getter)){
        return call_user_func(array($object, $getter));
      } else {
        throw new DataTable_DataTableException("$getter() method is required in " . get_class($object) . " or " . get_class($this));
      }
    }
  }

  /**
   * Render the default table HTML
   * 
   * @return string
   */
  protected function renderHtml()
  {
    $html = '';
    $html .= "<table cellpadding=\"0\" cellspacing=\"0\" border=\"0\" class=\"{$this->config->getClass()}\" id=\"{$this->getTableId()}\">";
    $html .= "<thead><tr>";

    foreach($this->config->getColumns() as $column){
        $html .= "<th>{$column->getTitle()}</th>";
    }

    $html .= "</tr></thead>";
    $html .= "<tbody>";

    if(!$this->config->isServerSideEnabled()){
      
      $html .= $this->renderStaticData();
      
    } else {
    
      $html .= "<tr><td class=\"dataTables_empty\">{$this->config->getLoadingHtml()}</td>";
    }
    
    $html .= "</tbody>";

    $html .= "<tfoot id=\"searchTFoot\" style=\"display: none;\"><tr>";    
    foreach($this->config->getColumns() as $column){
        //$value = $this->getDataForColumn($object, $column);
        $value = $column->getTitle();
        if($value == "") $type = "hidden";
        else $type="text";
                
        if($column->isVisible()){
          $html .="<th><input type='".$type."' name='search_".$value."' value='Search ".$value."' class='search_init' /></th>";                    
        } else {
          $html .="<th><input type='".$type."' name='search_".$value."' value='Search ".$value."' class='search_init' /></th>";          
        }
    }
    $html .= "</tr></tfoot>";
           
    $html .= "</table>";
        
    return $html;
  }

  /**
   * Render the table rows for a non-ajax datatable
   * 
   * @return string
   */
  protected function renderStaticData()
  {
    $data = $this->loadStaticData();

    $html = "";
    $color = new stdclass();
    
    foreach($data as $object){

      $row = "";

      foreach($this->config->getColumns() as $column){
        $value = $this->getDataForColumn($object, $column);
        if($columnt->getName() == "callid") {
              if(!isset($colors->{$value})) $colors->{$value} = "#".dechex(rand(128,255)).dechex(rand(128,255)).dechex(rand(128,255));        
              $style="padding: 0; margin: 0; border-top: 0; border-bottom: 1px solid #f5f5f5; border-left: 1px solid #f5f5f5; border-right: 1px solid #f5f5f5; color: #000000; font-size: 11px; background: ".$colors->{$value};
        }
        
        if($column->isVisible()){
          $row .= "<td style='".$style."'>{$value}</td>";
        } else {
          $row .= "<td style=\"display: none;\">{$value}</td>";
        }
      }
      
      /* if COLORTAG, color transaction base on CALLID, FROMTAG */
      //$callid = $row->callid . $row->from_tag;
      //if(strlen($callid) == 0) $callid="nonexist";
      //$mycolor = $colors->$callid;

      $html .= "<tr style='".$style."'>{$row}</tr>";
    }
    
    return $html;
  }

  /**
   * Call the implementing loadData() method to load static data
   * for a non-AJAX table
   * 
   * @return array
   */
  protected function loadStaticData()
  {
    // find the default sort column and direction from the config
    foreach($this->config->getColumns() as $index => $column){
      if($column->isDefaultSort()){
        $sortColumnIndex = $index;
        $sortDirection = $column->getDefaultSortDirection();
      }      
    }

    // make a fake request object
    $request = new DataTable_Request();
    $request->setDisplayStart(0);
    $request->setDisplayLength($this->config->getStaticMaxLength());
    $request->setSortColumnIndex($sortColumnIndex);
    $request->setSortDirection($sortDirection);

    // load data
    $dataResult = $this->loadData($request);

    // just return the entity array
    return $dataResult->getData();
  }
 
  /**
   * Render the DataTable instantiation javascript code
   * 
   * @return
   */
  protected function renderJs()
  {
    $js = "
			<script type=\"text/javascript\">
  			  var asInitVals = new Array();
  			  var initColors = new Array();
			  var oTable;
			  var giRedraw = false;
			  $(document).ready(function(){

				
				$('tfoot input').keyup( function () {
				    //{$this->getTableId()}.fnFilter( this.value, $('tfoot input').index(this) );
                                } );
                                $('tfoot input').each( function (i) {
                                    asInitVals[i] = this.value;
                                } );
	
                                $('tfoot input').focus( function () {
                                  if ( this.className == 'search_init' )
                                  {
                                    this.className = '';
                                    this.value = '';
                                  }
                                } );
	
                                $('tfoot input').change( function (i) {
                                if ( this.value != '' )
                                {
                                  {$this->getTableId()}.fnFilter( this.value, $('tfoot input').index(this) );
                                }
                                } );	

                                $('tfoot input').blur( function (i) {
                                if ( this.value == '' )
                                {
                                    this.className = 'search_init';                                    
                                    this.value = asInitVals[$('tfoot input').index(this)];
                                }
				else {
                                  {$this->getTableId()}.fnFilterClear();
                                }
                                } );	                                                 
                                
				$('#{$this->getTableId()} tbody').click(function(event) {
					$(oTable.fnSettings().aoData).each(function (){
						$(this.nTr).removeClass('row_selected');
					});
					$(event.target.parentNode).addClass('row_selected');
					//$(event.target.parentNode).css('backgroung-color','#ff00');
				});
	
				$('#delete').click( function() {
					var anSelected = fnGetSelected( oTable );
					{$this->getTableId()}.fnDeleteRow( anSelected[0] );
				} );	

				var {$this->getTableId()} = $('#{$this->getTableId()}').dataTable({$this->renderDataTableOptions()});
				oTable = {$this->getTableId()};
                                               
			    });
			</script>
		";

    return $js;
  }

  /**
   * Convert all the DataTable_Config options into a javascript array string
   * 
   * @return string
   */
  protected function renderDataTableOptions()
  {
    $options = array();

    $options["bPaginate"]       = $this->config->isPaginationEnabled();
    $options["bLengthChange"] 	= $this->config->isLengthChangeEnabled();
    $options["bProcessing"] 	= $this->config->isProcessingEnabled();
    $options["bFilter"]         = $this->config->isFilterEnabled();
    $options["bSort"] 	    	= $this->config->isSortEnabled();
    $options["bInfo"] 	      	= $this->config->isInfoEnabled();
    $options["bAutoWidth"]      = $this->config->isAutoWidthEnabled();
    $options["bScrollCollapse"]	= $this->config->isScrollCollapseEnabled();
    $options["bScrollInfinite"]	= $this->config->isScrollInfiniteEnabled();
    $options["iDisplayLength"] 	= $this->config->getDisplayLength();
    $options["bJQueryUI"]       = $this->config->isJQueryUIEnabled();
    $options["sPaginationType"]	= $this->config->getPaginationType();    

    $options["bStateSave"]      = $this->config->isSaveStateEnabled();
    $options["iCookieDuration"] = $this->config->getCookieDuration();
    $options["asStripClasses"]  = $this->config->getStripClasses();
    
    //setStripClasses
    
    $options["aoColumns"]       = $this->renderDataTableColumnOptions();
    $options["aaSorting"]     	= $this->renderDefaultSortColumns();
    $options["aLengthMenu"] 	= $this->renderLengthMenu();
    $options["bSortClasses"] 	= false;
    
    if($this->config->isServerSideEnabled()){
      $options["bServerSide"] 	= $this->config->isServerSideEnabled();
      $options["sAjaxSource"] 	= $this->getAjaxSource();
    }  
    
    if(!is_null($this->config->getScrollX())){
      $options["sScrollX"] = $this->config->getScrollX();
    }

    if(!is_null($this->config->getScrollY())){
      $options["sScrollY"] = $this->config->getScrollY();
    }
    
    if(!is_null($this->config->getScrollLoadGap())){
      $options["iScrollLoadGap"] = $this->config->getScrollLoadGap();
    }
    
    if(!is_null($this->config->getLanguageConfig())){
      $options["oLanguage"]	= $this->renderLanguageConfig();
    }
      
    if(!is_null($this->config->getCookiePrefix())){
      $options["sCookiePrefix"]	= $this->config->getCookiePrefix();
    }
    
    if(!is_null($this->config->getDom())){
      $options["sDom"] = $this->config->getDom();
    }
    
    // =====================================================================================
    // add callback functions
    // =====================================================================================
    if(!is_null($this->getRowCallbackFunction())){
      $options["fnRowCallback"] = $this->getCallbackFunctionProxy('getRowCallbackFunction');
    }
    
    if(!is_null($this->getServerDataFunction())){
      $options["fnServerData"] = $this->getCallbackFunctionProxy('getServerDataFunction');
    }

    if(!is_null($this->getInitCompleteFunction())){
      $options["fnInitComplete"] = $this->getCallbackFunctionProxy('getInitCompleteFunction');
    } 
    
    if(!is_null($this->getDrawCallbackFunction())){
      $options["fnDrawCallback"] = $this->getCallbackFunctionProxy('getDrawCallbackFunction');
    } 
    
    if(!is_null($this->getFooterCallbackFunction())){
      $options["fnFooterCallback"] = $this->getCallbackFunctionProxy('getFooterCallbackFunction');
    } 
    
    if(!is_null($this->getFooterCallbackFunction())){
      $options["fnHeaderCallback"] = $this->getCallbackFunctionProxy('getHeaderCallbackFunction');
    }  
    
    if(!is_null($this->getInfoCallbackFunction())){
      $options["fnInfoCallback"] = $this->getCallbackFunctionProxy('getInfoCallbackFunction');
    } 
    
    if(!is_null($this->getCookieCallbackFunction())){
      $options["fnCookieCallback"] = $this->getCallbackFunctionProxy('getCookieCallbackFunction');
    } 
    
    // build the initial json object
    $json = json_encode($options);
    
    // replace keys for functions with actual functions
    $json = $this->replaceJsonFunctions($json);
    
    return $json;    
  }

  /**
   * This method replaces any keys within the given json string 
   * that were created in getCallbackFunctionProxy
   * 
   * This essentially is a hack to make sure that the functions
   * don't have double quotes around them which keeps javascript
   * from interpreting them as functions.
   * 
   * @param string $json
   */
  protected function replaceJsonFunctions($json)
  {
    foreach($this->jsonFunctions as $key => $function){
      
      $search = '"' . $key . '"';
      
      $json = str_replace($search, $function, $json);
    }
    
    return $json;
  }
  
  /**
   * Proxy method to call the current object's getRowCallBackFunction() method
   * and clean up the result for the javascript.
   * 
   * This method also creates a unique key for each function which it returns
   * for later lookup against the function stored in $this->jsonFunctions
   * 
   * @return string
   */
  protected function getCallbackFunctionProxy($function)
  {
    // get the js function string
    $js = call_user_func(array($this, $function));

    $jsonKey = $this->buildJsonFunctionKey($js);
    
    return $jsonKey;
  }


  /**
   * Build a unique key for the given javascript function
   * and store they key => function in the local jsonFunctions
   * variable.
   * 
   * This key will get used later in replaceFunctions to replace
   * the key with the actual function to fix the final json string.
   * 
   * @return string
   */ 
  protected function buildJsonFunctionKey($js)
  {
    // remove comments
    $js = preg_replace('!/\*.*?\*/!s', '', $js);  // removes /* comments */
    $js = preg_replace('!//.*?\n!', '', $js); // removes //comments
   
    // remove all extra whitespace
    $js = str_replace(array("\t", "\n", "\r\n"), '', trim($js));
     
    // build a temporary key
    $jsonKey = md5($js);
    
    // store key => function mapping
    $this->jsonFunctions[$jsonKey] = $js;
    
    return $jsonKey;
  }
  
  /**
   * Build the array for the 'aoColumns' DataTable option
   * 
   * @return array
   */
  protected function renderDataTableColumnOptions()
  {
    $columns = array();

    foreach($this->config->getColumns() as $column){

      $tempColumn = array(
				"bSortable" => $column->isSortable(),
				"sName" => $column->getName(),
				"bVisible" => $column->isVisible(),
                "bSearchable" => $column->isSearchable(),
      );

      if(!is_null($column->getWidth())){
        $tempColumn['sWidth'] = $column->getWidth();
      }

      if(!is_null($column->getClass())){
        $tempColumn['sClass'] = $column->getClass();
      }

      if(!is_null($column->getRenderFunction())){
        $tempColumn['fnRender'] = $this->buildJsonFunctionKey($column->getRenderFunction());
      }
      
      $columns[] = $tempColumn;
    }

    return $columns;
  }

  /**
   * Build the array for the 'aaSorting' option
   * 
   * @return array
   */
  protected function renderDefaultSortColumns()
  {
    $columns = array();

    foreach($this->config->getColumns() as $id => $column){
      if($column->isDefaultSort()){
        $columns[] = array($id, $column->getDefaultSortDirection());
      }
    }

    return $columns;
  }

  /**
   * Build the array for the 'aLengthMenu' option
   * 
   * @return array
   */
  protected function renderLengthMenu()
  {
    return array(array_keys($this->config->getLengthMenu()), array_values($this->config->getLengthMenu()));
  }

  /**
   * Build the array for the 'oLanguage' option from the LanguageConfig object
   * 
   * @return array
   */
  protected function renderLanguageConfig()
  {
    $options = array();

    $paginate = array();

    if(!is_null($this->config->getLanguageConfig()->getPaginateFirst())){
	  $paginate["sFirst"] = $this->config->getLanguageConfig()->getPaginateFirst();
    }

    if(!is_null($this->config->getLanguageConfig()->getPaginateLast())){
	  $paginate["sLast"] = $this->config->getLanguageConfig()->getPaginateLast();
    }

    if(!is_null($this->config->getLanguageConfig()->getPaginateNext())){
	  $paginate["sNext"] = $this->config->getLanguageConfig()->getPaginateNext();
    }

    if(!is_null($this->config->getLanguageConfig()->getPaginatePrevious())){
	  $paginate["sPrevious"] = $this->config->getLanguageConfig()->getPaginatePrevious();
    }

    // add oPaginate to options if anything was set for object
    if(count($paginate) > 0){
      $options["oPaginate"] = $paginate;
    }

    if(!is_null($this->config->getLanguageConfig()->getEmptyTable())){
	  $options["sEmptyTable"] = $this->config->getLanguageConfig()->getEmptyTable();
    }
    
    if(!is_null($this->config->getLanguageConfig()->getInfo())){
	  $options["sInfo"] = $this->config->getLanguageConfig()->getInfo();
    }
      
    if(!is_null($this->config->getLanguageConfig()->getInfoEmpty())){
	  $options["sInfoEmpty"] = $this->config->getLanguageConfig()->getInfoEmpty();
    }
      
    if(!is_null($this->config->getLanguageConfig()->getInfoFiltered())){
	  $options["sInfoFiltered"] = $this->config->getLanguageConfig()->getInfoFiltered();
    }
     
    if(!is_null($this->config->getLanguageConfig()->getInfoPostFix())){
	  $options["sInfoPostFix"] = $this->config->getLanguageConfig()->getInfoPostFix();
    }
    
    if(!is_null($this->config->getLanguageConfig()->getLengthMenu())){
	  $options["sLengthMenu"] = $this->config->getLanguageConfig()->getLengthMenu();
    }
      
    if(!is_null($this->config->getLanguageConfig()->getSearch())){
	  $options["sSearch"] = $this->config->getLanguageConfig()->getSearch();
    }

    if(!is_null($this->config->getLanguageConfig()->getZeroRecords())){
	  $options["sZeroRecords"] = $this->config->getLanguageConfig()->getZeroRecords();
    }
    
    if(!is_null($this->config->getLanguageConfig()->getUrl())){
	  $options["sUrl"] = $this->config->getLanguageConfig()->getUrl();
    }

    return $options;
  }

  /**
   * Set the ajax source url for the current object
   * 
   * This overrides the value that may have been set on
   * the DataTable_Config object
   * 
   * @param string $ajaxDataUrl
   */
  public function setAjaxDataUrl($ajaxDataUrl)
  {
    $this->ajaxDataUrl = $ajaxDataUrl;
  }
  
  public function setSearchRequest($searchData)
  {
    $this->searchRequest = $searchData;
  }

  /**
   * Get the ajax source url that was set either on the DataTable_Config
   * object or on the current DataTable object
   * 
   * @return string
   */
  public function getAjaxSource()
  {
    if(!is_null($this->config->getAjaxSource())){
      return $this->config->getAjaxSource();
    } else {
      return $this->ajaxDataUrl;
    }
  }

  /**
   * Utility method to find a column positon index
   * by the column's name
   *
   * @return integer
   */
  protected function getColumnIndexByName($name)
  {
    if(is_null($this->columnIndexNameCache)){
      $this->buildColumnIndexNameCache();
    }

    return $this->columnIndexNameCache[$name];
  }


  public function getColumnIndexByNumber($number)
  {
      foreach($this->config->getColumns() as $column){        
        if($column->isVisible()){
            $cols[] = $column->getName();       
        }
    }
     return $cols[$number];
  }

  /**
   * Utility method to get all the column names 
   * that are configured as being searchable
   * 
   * @return array
   */
  protected function getSearchableColumnNames()
  {
    $cols = array();
    
    foreach($this->config->getColumns() as $column){
      if($column->isSearchable()){
        $cols[] = $column->getName();
      }
    }
    
    return $cols;
  }
  
  /**
   * Build an array of Column->name => position index
   * for quick lookups
   *
   * @return void
   */
  protected function buildColumnIndexNameCache()
  {
    $this->columnIndexNameCache = array();

    foreach($this->config->getColumns() as $index => $column){
      $this->columnIndexNameCache[$column->getName()] = $index;
    }
  }
}