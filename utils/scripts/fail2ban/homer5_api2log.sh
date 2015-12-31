#!/bin/bash
# Get Alarms in timerange from HOMER 5 API (http://sipcapture.org)
# (C) 2015 QXIP BV
#
# fail2ban filter:
#
# 	[Definition]
# 	failregex = ^.* scanner: (?P<counter>\d*) <HOST>.*

# OPTIONS:
# TIMERANGE
	MINUTES=10
	FROMTIME=$(date +%s%N -d "$MINUTES minutes ago" | cut -b1-13)
	TOTIME=$(date +%s%N | cut -b1-13)
# HOMER API
	APIURL="http://127.0.0.1/api"
	USER="admin"
	PASS="pass1234"

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
echo "Authorizing..."
AUTH=$(curl -s --cookie "HOMERSESSID=tcuass65ejl2lifoopuuurpmq7; path=/" -X POST -H "Content-Type: application/json" -d '{"username":"'$USER'","password":"'$PASS'"}' $APIURL/v1/session)

# Get REGISTER Statistics
ALARMLIST=$(curl -s --cookie "HOMERSESSID=tcuass65ejl2lifoopuuurpmq7; path=/" -X POST -H "Content-Type: application/json" -d '{"timestamp":{"from":'$FROMTIME',"t":'$TOTIME'},"param":{"filter":[],"total":false}}' $APIURL/v1/alarm/list/get)
# echo "$ALARMLIST"

TOTAL=$(echo "$ALARMLIST"|jq '.count')

# get totals
TOTALS=$(echo "$ALARMLIST" | jq -r '.data[] .total' )
if [ -z "$TOTALS" ]; then
	echo "No results"
	exit
fi

# calculate averages
echo "$TOTALS" | awk '{a[i++]=$0;s+=$0}END{print "min:"a[0],"max:"a[i-1],"median:"(a[int(i/2)]+a[int((i-1)/2)])/2,"mean:"s/i}'

# Turn alarms into loglines for fail2ban
	COUNTER=0
         while [  $COUNTER -ne "$TOTAL" ]; do
		     echo "$ALARMLIST" | jq -r '.data['$COUNTER'] | "\(.create_date) \(.type): \(.total) \(.source_ip) [\(.description)]"'
             let COUNTER=COUNTER+1
         done
