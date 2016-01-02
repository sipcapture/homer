#!/bin/bash
# Set iptables rules based on detections from HOMER 5 Alarms API (http://sipcapture.org)
# (C) 2015 QXIP BV

# OPTIONS:
# TIMERANGE
	MINUTES=10
	FROMTIME=$(date +%s%N -d "$MINUTES minutes ago" | cut -b1-13)
	TOTIME=$(date +%s%N | cut -b1-13)
# SIP METHOD
	METHOD="REGISTER"
# HOMER API
	APIURL="http://127.0.0.1/api"
	USER="admin"
	PASS="pass1234"
# IPTABLES
  IPTABLES="/sbin/iptables"
  IPRULE="DROP"
# WHITELIST
  WHITELIST=('127.0.0.1' '192.168.1.1')
  
# USER INPUT
while [ "$1" != "" ]; do
case $1 in
    -a | -api )
        APIURL=$2
        if [ "$APIURL" = "" ]; then echo "Error: Missing API URL"; exit 1; fi
        ;;

    -u | -user )
        USER=$2
        if [ "$USER" = "" ]; then
            echo "Error: Missing API Username"
            exit 1
        fi
        ;;

    -p | -pass )
        PASS=$2
        if [ "$PASS" = "" ]; then
            echo "Error: Missing API Password"
            exit 1
        fi
        ;;

    -l | -last )
        MINUTES=$2
        if [ "$MINUTES" = "" ]; then
            echo "Error: No time defined"
            exit 1
        fi
	FROMTIME=$(date +%s%N -d "$MINUTES minutes ago" | cut -b1-13)
	TOTIME=$(date +%s%N | cut -b1-13)
        ;;

    -h | -help )
        echo "Options:"
        echo "  -l |-last	Search Last X Minutes"
        echo "  -a |-api	Homer5 API URL"
        echo "  -u |-user	Homer5 API Uusername"
        echo "  -p |-pass	Homer5 API Password"
	echo
        exit 0
        ;;

     *)
        echo "Error: Unknown option $2"
        exit 1
        ;;

esac
shift 2
done

# Authorize Client
echo "Checking for Alarms in last $MINUTES..."
AUTH=$(curl -s --cookie "HOMERSESSID=tcuass65ejl2lifoopuuurpmq7; path=/" -X POST -H "Content-Type: application/json" -d '{"username":"'$USER'","password":"'$PASS'"}' $APIURL/v1/session)

# Get REGISTER Statistics
ALARMLIST=$(curl -s --cookie "HOMERSESSID=tcuass65ejl2lifoopuuurpmq7; path=/" -X POST -H "Content-Type: application/json" -d '{"timestamp":{"from":'$FROMTIME',"t":'$TOTIME'},"param":{"filter":[],"total":false}}' $APIURL/v1/alarm/list/get)

# echo "$ALARMLIST"

TOTAL=$(echo "$ALARMLIST"|jq '.count')

# get totals
TOTALS=$(echo "$ALARMLIST" | jq '.data[] .total' | tr -d '"' )
if [ -z "$TOTALS" ]; then
	echo "No results"
	exit
fi
# echo "$TOTALS"
offenders=()

# Process Alarms to iptables DROP rules
	COUNTER=0
         while [  $COUNTER -ne "$TOTAL" ]; do
		     THISIP=$(echo "$ALARMLIST" | jq -r '.data['$COUNTER'] | "\(.source_ip)"')
		     if [[ ! " ${offenders[@]} " =~ " ${THISIP} " ]]; then
			# echo "Adding $THISIP"
			offenders=("${Unix[@]}" "$THISIP")
		     fi
             let COUNTER=COUNTER+1
         done

  # Check/Create chain
  if [ `iptables -L | grep -c "Chain H5BLACKLIST-INPUT"` -lt 1 ]; then
    /sbin/iptables -N H5BLACKLIST-INPUT
    /sbin/iptables -I INPUT 1 -j H5BLACKLIST-INPUT
  fi

	for ip in "${offenders[@]}"
	do
	   :
	   if [[ ! " ${WHITELIST[@]} " =~ " ${ip} " ]]; then
	   	echo "Banning $ip"
	   	echo "$IPTABLES -A H5BLACKLIST-INPUT -s \"$ip\" -j $IPRULE" | sh
	   fi
	done

echo "Done!"
