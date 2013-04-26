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
 * This class represents a collection of DataTable_Column objects
 * 
 * @package DataTable
 * @author	Marc Roulias <marc@lampjunkie.com>
 */
class DataTable_ColumnCollection implements Iterator
{
  protected $items = array();
  protected $index = 0;

  public function __construct($items = array())
  {
    $this->items = $items;
  }

  public function add(DataTable_Column $item)
  {
    $this->items[] = $item;
    return $this;
  }

  public function get($index)
  {
    return $this->items[$index];
  }

  public function count()
  {
    return count($this->items);
  }

  public function rewind()
  {
    $this->index = 0;
  }

  public function current()
  {
    return $this->items[$this->index];
  }

  public function key()
  {
    return $this->index;
  }

  public function next()
  {
    ++$this->index;
  }

  public function valid()
  {
    return isset($this->items[$this->index]);
  }
}
