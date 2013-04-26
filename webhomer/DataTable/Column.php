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
 * This class holds the configuration values for a column in a DataTable
 *
 * @package DataTable
 * @author	Marc Roulias <marc@lampjunkie.com>
 */
class DataTable_Column
{
  /**
   * The name of the column
   * @var string
   */
  protected $name;

  /**
   * The title for the column
   *
   * This value is used in the rendering of the table's <thead>
   *
   * @var string
   */
  protected $title;

  /**
   * The name of the getter method that should be called on either
   * each of the row data objects or on the implementing DataTable class
   *
   * @var string
   */
  protected $getMethod;

  /**
   * The key that should be used for sorting purposes
   *
   * @var string
   */
  protected $sortKey;

  /**
   * Is this column visible?
   * @var boolean
   */
  protected $isVisible = true;

  /**
   * Is this column sortable?
   * @var boolean
   */
  protected $isSortable = false;

  /**
   * Is this the default sort column?
   * @var boolean
   */
  protected $isDefaultSort = false;

  /**
   * Is this column searchable
   * @var boolean
   */
  protected $isSearchable = false;

  /**
   * The default sort direction if this is the default sort column
   *
   * Should be either 'asc' or 'desc'
   *
   * @var string
   */
  protected $defaultSortDirection = 'asc';

  /**
   * The fixed width of this column
   *
   * @var string
   */
  protected $width;

  /**
   * The CSS class to apply to all cells in this column
   *
   * @var string
   */
  protected $class;


  protected $renderFunction;

  public function setName($name)
  {
    $this->name = $name;
    return $this;
  }

  public function getName()
  {
    return $this->name;
  }

  public function setTitle($title)
  {
    $this->title = $title;
    return $this;
  }

  public function getTitle()
  {
    return $this->title;
  }

  public function setGetMethod($getMethod)
  {
    $this->getMethod = $getMethod;
    return $this;
  }

  public function getGetMethod()
  {
    return $this->getMethod;
  }

  public function setSortKey($sortKey)
  {
    $this->sortKey = $sortKey;
    return $this;
  }

  public function getSortKey()
  {
    return $this->sortKey;
  }

  public function isVisible()
  {
    return $this->isVisible;
  }

  public function setIsVisible($isVisible)
  {
    $this->isVisible = $isVisible;
    return $this;
  }

  public function setIsSortable($isSortable)
  {
    $this->isSortable = $isSortable;
    return $this;
  }

  public function isSortable()
  {
    return $this->isSortable;
  }

  public function setIsDefaultSort($isDefaultSort)
  {
    $this->isDefaultSort = $isDefaultSort;
    return $this;
  }

  public function isDefaultSort()
  {
    return $this->isDefaultSort;
  }

  public function setDefaultSortDirection($defaultSortDirection)
  {
    $this->defaultSortDirection = $defaultSortDirection;
    return $this;
  }

  public function getDefaultSortDirection()
  {
    return $this->defaultSortDirection;
  }

  public function setWidth($width)
  {
    $this->width = $width;
    return $this;
  }

  public function getWidth()
  {
    return $this->width;
  }

  public function setClass($class)
  {
    $this->class = $class;
    return $this;
  }

  public function getClass()
  {
    return $this->class;
  }

  public function isSearchable()
  {
    return $this->isSearchable;
  }

  public function setIsSearchable($isSearchable)
  {
    $this->isSearchable = $isSearchable;
    return $this;
  }

  public function getRenderFunction()
  {
    return $this->renderFunction;
  }

  public function setRenderFunction($renderFunction)
  {
    $this->renderFunction = $renderFunction;
    return $this;
  }
}
