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
 * Autoloads DataTable classes
 * 
 * @package DataTable
 * @author	Marc Roulias <marc@lampjunkie.com>
 */
class DataTable_Autoloader
{
    /**
     * Registers this as an SPL autoloader.
     */
    static public function register()
    {
        ini_set('unserialize_callback_func', 'spl_autoload_call');
        spl_autoload_register(array(new self, 'autoload'));
    }

    /**
     * Handles autoloading of classes.
     *
     * @param  string  $class  A class name.
     *
     * @return boolean Returns true if the class has been loaded
     */
    static public function autoload($class)
    {
        if (0 !== strpos($class, 'DataTable')) {
            return;
        }

        if (file_exists($file = dirname(__FILE__).'/../'.str_replace('_', '/', $class).'.php')) {
            require $file;
        }
    }
}
