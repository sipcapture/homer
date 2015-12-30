#!/bin/bash
# Example Script for HOMER 5 API (http://sipcapture.org)
# (C) 2015 QXIP BV
#
# * Calculate IPs beyond 95th percentile threshold in timerange
#
# Usage Example:
#  ./example.sh -l 60 -m REGISTER -a http://1.2.3.4/api -u someuser -p somepass

# SCRIPT DEFAULTS:
	MINUTES=10
	FROMTIME=$(date +%s%N -d "$MINUTES minutes ago" | cut -b1-13)
	TOTIME=$(date +%s%N | cut -b1-13)
	METHOD="REGISTER"
	APIURL="http://127.0.0.1/api"
	USER="admin"
	PASS="pass1234"

# USER OPTIONS
while [ "$1" != "" ]; do
case $1 in
    -a | -api )
        APIURL=$2
        if [ "$APIURL" = "" ]; then echo "Error: Missing API URL"; exit 1; fi
        ;;

    -u | -user )
        USER=$2
        if [ "$USER" = "" ]; then echo "Error: Missing API Username"; exit 1; fi
        ;;

    -p | -pass )
        PASS=$2
        if [ "$PASS" = "" ]; then echo "Error: Missing API Password"; exit 1; fi
        ;;

    -m | -method )
        METHOD=$2
        if [ "$METHOD" = "" ]; then echo "Error: Missing SIP Method"; exit 1; fi
        ;;

    -l | -last )
        MINUTES=$2
        if [ "$MINUTES" = "" ]; then echo "Error: No time defined"; exit 1; fi
      	FROMTIME=$(date +%s%N -d "$MINUTES minutes ago" | cut -b1-13)
	      TOTIME=$(date +%s%N | cut -b1-13)
        ;;

    -h | -help )
        echo "Options:"
        echo "  -l |-last	Search Last X Minutes"
        echo "  -m |-method	Filter by SIP Method"
        echo "  -a |-api	Homer5 API URL"
        echo "  -u |-user	Homer5 API Uusername"
        echo "  -p |-pass	Homer5 API Password"
	echo
        exit 0
        ;;

     *)
        echo "Error: Unknown option $2"; exit 1
        ;;

esac
shift 2
done


# Authorize Client
echo "Authorizing $METHOD in last $MINUTES..."
AUTH=$(curl -s --cookie "HOMERSESSID=tcuass65ejl2lifoopuuurpmq7; path=/" -X POST -H "Content-Type: application/json" -d '{"username":"'$USER'","password":"'$PASS'"}' $APIURL/v1/session)

# Get REGISTER Statistics
REGSTAT=$(curl -s --cookie "HOMERSESSID=tcuass65ejl2lifoopuuurpmq7; path=/" -X POST -H "Content-Type: application/json" -d '{"timestamp":{"from":'$FROMTIME',"t":'$TOTIME'},"param":{"filter":[{"method":"'$METHOD'"}],"total":true}}' $APIURL/v1/statistic/ip)

TOTAL=$(echo "$REGSTAT"|jq '.count')

# get totals
TOTALS=$(echo "$REGSTAT" | jq '.data[] .total' | tr -d '"' | sort -n )
if [ -z "$TOTALS" ]; then
	echo "No results"
	exit
fi

# calculate averages
echo "$TOTALS" | awk '{a[i++]=$0;s+=$0}END{print "min:"a[0],"max:"a[i-1],"median:"(a[int(i/2)]+a[int((i-1)/2)])/2,"mean:"s/i}'

# 95th percentile
PERC95=$(echo "$TOTALS" | awk 'BEGIN{c=0} length($0){a[c]=$0;c++}END{p5=(c/100*5); p5=p5%1?int(p5)+1:p5; print a[c-p5-1]}')

# Check entries
	COUNTER=0
         while [  $COUNTER -ne "$TOTAL" ]; do
	     THISTOT=$(echo "$REGSTAT" | jq '.data['$COUNTER'] .total' | tr -d '"')
	     if [ "$THISTOT" -gt "$PERC95" ]; then
		     echo "$REGSTAT" | jq '.data['$COUNTER'] .source_ip'
		     echo "$REGSTAT" | jq '.data['$COUNTER'] .total'
		     echo "$REGSTAT" | jq '.data['$COUNTER'] .method'
	     fi
             let COUNTER=COUNTER+1 
         done
