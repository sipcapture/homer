<?php

interface ISipService
{
  public function getAll($offset, $num, $sort, $sortDirection = 'asc', $isCount = false, $homer);
  public function searchAll($search, $columns, $offset, $num, $sort, $sortDirection = 'asc', $isCount = false, $homer, $searchColumns, $parent);
}