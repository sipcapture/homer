DROP TABLE stats_method;
CREATE TABLE IF NOT EXISTS `stats_method` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `date` timestamp NOT NULL DEFAULT '0000-00-00 00:00:00',
  `method` varchar(50) NOT NULL DEFAULT '',
  `total` int(10) NOT NULL DEFAULT 0,
  `auth` int(10) NOT NULL DEFAULT 0,
  `completed` int(10) NOT NULL DEFAULT 0,
  `uncompleted` int(10) NOT NULL DEFAULT 0,
  `rejected` int(10) NOT NULL DEFAULT 0,
  `asr` int(10) NOT NULL DEFAULT 0,
  `ner` int(10) NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `date` (`date`),
  KEY `method` (`method`),
  KEY `total` (`total`),
  KEY `completed` (`completed`),
  KEY `uncompleted` (`uncompleted`),
  UNIQUE KEY `datemethod` (`date`,`method`)
) ENGINE=MyISAM DEFAULT CHARSET=latin1;


DROP TABLE stats_useragent;
CREATE TABLE IF NOT EXISTS `stats_useragent` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `date` timestamp NOT NULL DEFAULT '0000-00-00 00:00:00',
  `useragent` varchar(100) NOT NULL DEFAULT '',
  `method` varchar(50) NOT NULL DEFAULT '',
  `total` int(10) NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  KEY `date` (`date`),
  KEY `useragent` (`useragent`),
  KEY `method` (`method`),
  KEY `total` (`total`),
  UNIQUE KEY `datemethodua` (`date`,`method`,`useragent`)
) ENGINE=MyISAM DEFAULT 