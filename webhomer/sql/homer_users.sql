
CREATE TABLE IF NOT EXISTS `homer_hosts` (
  `id` int(10) NOT NULL AUTO_INCREMENT,
  `host` varchar(80) NOT NULL,
  `name` varchar(100) NOT NULL,
  `status` tinyint(1) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `id` (`id`),
  UNIQUE KEY `host_2` (`host`),
  KEY `host` (`host`)
) ENGINE=MyISAM  DEFAULT CHARSET=utf8 AUTO_INCREMENT=21 ;

--
-- Daten f Tabelle `homer_hosts`
--

INSERT INTO `homer_hosts` VALUES(1, '192.168.0.30', 'proxy01', 1);
INSERT INTO `homer_hosts` VALUES(2, '192.168.0.4', 'acme-234', 1);

-- --------------------------------------------------------

--
-- Tabellenstruktur f Tabelle `homer_logon`
--

CREATE TABLE IF NOT EXISTS `homer_logon` (
  `userid` int(11) NOT NULL AUTO_INCREMENT,
  `useremail` varchar(50) NOT NULL DEFAULT '',
  `password` varchar(50) NOT NULL DEFAULT '',
  `userlevel` int(11) NOT NULL DEFAULT '0',
  PRIMARY KEY (`userid`)
) ENGINE=MyISAM  DEFAULT CHARSET=latin1 AUTO_INCREMENT=2 ;

--
-- Daten f Tabelle `homer_logon`
--

INSERT INTO `homer_logon` VALUES(NULL, 'test@test.com', MD5('test123'), 1);

-- --------------------------------------------------------

--
-- Tabellenstruktur f Tabelle `homer_nodes`
--

CREATE TABLE IF NOT EXISTS `homer_nodes` (
  `id` int(10) NOT NULL AUTO_INCREMENT,
  `host` varchar(80) NOT NULL,
  `name` varchar(100) NOT NULL,
  `status` tinyint(1) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `id` (`id`),
  UNIQUE KEY `host_2` (`host`),
  KEY `host` (`host`)
) ENGINE=MyISAM  DEFAULT CHARSET=utf8 AUTO_INCREMENT=21 ;

--
-- Daten f Tabelle `homer_nodes`
--

INSERT INTO `homer_nodes` VALUES(1, '10.0.81.145', 'node01', 1);
INSERT INTO `homer_nodes` VALUES(2, '10.0.136.234', 'node02', 2);
