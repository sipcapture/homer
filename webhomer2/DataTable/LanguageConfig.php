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
 * This class holds the language configuration values for a DataTable
 * 
 * @package DataTable
 * @author	Marc Roulias <marc@lampjunkie.com>
 */
class DataTable_LanguageConfig
{
  protected $paginateFirst;
  protected $paginateLast;
  protected $paginateNext;
  protected $paginatePrevious;
  protected $emptyTable;
  protected $info;
  protected $infoEmpty;
  protected $infoFiltered;
  protected $infoPostFix;
  protected $lengthMenu;
  protected $search;
  protected $url;
  protected $zeroRecords;

  public function getPaginateFirst()
  {
    return $this->paginateFirst;
  }

  public function setPaginateFirst($paginateFirst)
  {
    $this->paginateFirst = $paginateFirst;
    return $this;
  }

  public function getPaginateLast()
  {
    return $this->paginateLast;
  }

  public function setPaginateLast($paginateLast)
  {
    $this->paginateLast = $paginateLast;
    return $this;
  }

  public function getPaginateNext()
  {
    return $this->paginateNext;
  }

  public function setPaginateNext($paginateNext)
  {
    $this->paginateNext = $paginateNext;
    return $this;
  }

  public function getPaginatePrevious()
  {
    return $this->paginatePrevious;
  }

  public function setPaginatePrevious($paginatePrevious)
  {
    $this->paginatePrevious = $paginatePrevious;
    return $this;
  }

  public function getEmptyTable()
  {
    return $this->emptyTable;
  }

  public function setEmptyTable($emptyTable)
  {
    $this->emptyTable = $emptyTable;
    return $this;
  }

  public function getInfo()
  {
    return $this->info;
  }

  public function setInfo($info)
  {
    $this->info = $info;
    return $this;
  }

  public function getInfoEmpty()
  {
    return $this->infoEmpty;
  }

  public function setInfoEmpty($infoEmpty)
  {
    $this->infoEmpty = $infoEmpty;
    return $this;
  }

  public function getInfoFiltered()
  {
    return $this->infoFiltered;
  }

  public function setInfoFiltered($infoFiltered)
  {
    $this->infoFiltered = $infoFiltered;
    return $this;
  }

  public function getInfoPostFix()
  {
    return $this->infoPostFix;
  }

  public function setInfoPostFix($infoPostFix)
  {
    $this->infoPostFix = $infoPostFix;
    return $this;
  }

  public function getLengthMenu()
  {
    return $this->lengthMenu;
  }

  public function setLengthMenu($lengthMenu)
  {
    $this->lengthMenu = $lengthMenu;
    return $this;
  }

  public function getSearch()
  {
    return $this->search;
  }

  public function setSearch($search)
  {
    $this->search = $search;
    return $this;
  }

  public function getUrl()
  {
    return $this->url;
  }

  public function setUrl($url)
  {
    $this->url = $url;
    return $this;
  }

  public function getZeroRecords()
  {
    return $this->zeroRecords;
  }

  public function setZeroRecords($zeroRecords)
  {
    $this->zeroRecords = $zeroRecords;
    return $this;
  }
}