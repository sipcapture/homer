#!/bin/sh

programm="/usr/local/bin/partrotate_unixtimestamp.pl"

#sip_capture: part step: 1 hour and 10 days keep data
$programm sip_capture 1 10

#rtcp_capture: part step: 1 day and 10 days keep data.
$programm rtcp_capture 0 10

#logs_capture: part step: 1 day and 10 days keep data.
$programm logs_capture 0 10

#stats: part step: 1 day and 20 days keep data
$programm stats_ip 0 20

