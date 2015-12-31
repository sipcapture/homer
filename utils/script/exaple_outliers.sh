#!/bin/bash
# Get Series Anomaly detections from HOMER 5 API (http://sipcapture.org)
# (C) 2015 QXIP BV

# OPTIONS:
	MINUTES=10
	FROMTIME=$(date +%s%N -d "$MINUTES minutes ago" | cut -b1-13)
	TOTIME=$(date +%s%N | cut -b1-13)
	METHOD="REGISTER"
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

    -m | -method )
        METHOD=$2
        if [ "$METHOD" = "" ]; then
            echo "Error: Missing SIP Method"
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
        echo "  -m |-method	Filter by SIP Method"
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
echo "Authorizing $METHOD in last $MINUTES..."
AUTH=$(curl -s --cookie "HOMERSESSID=tcuass65ejl2lifoopuuurpmq7; path=/" -X POST -H "Content-Type: application/json" -d '{"username":"'$USER'","password":"'$PASS'"}' $APIURL/v1/session)

# Get REGISTER Statistics
REGSTAT=$(curl -s --cookie "HOMERSESSID=tcuass65ejl2lifoopuuurpmq7; path=/" -X POST -H "Content-Type: application/json" -d '{"timestamp":{"from":'$FROMTIME',"t":'$TOTIME'},"param":{"filter":[{"method":"'$METHOD'"}],"total":false}}' $APIURL/v1/statistic/ip)

# get entries and totals
TOTAL=$(echo "$REGSTAT"|jq '.count')
TOTALS=$(echo "$REGSTAT" | jq -r '.data[] .total' )
if [ -z "$TOTALS" ]; then
	echo "No results"
	exit
fi
# echo "$TOTALS"

# calculate averages
echo "$TOTALS" | awk '{a[i++]=$0;s+=$0}END{print "min:"a[0],"max:"a[i-1],"median:"(a[int(i/2)]+a[int((i-1)/2)])/2,"mean:"s/i}'

# percentile steps
PERC95=$(echo "$TOTALS" | awk 'BEGIN{c=0} length($0){a[c]=$0;c++}END{p5=(c/100*5); p5=p5%1?int(p5)+1:p5; print a[c-p5-1]}')
PERC75=$(echo "$TOTALS" | awk 'BEGIN{c=0} length($0){a[c]=$0;c++}END{p5=(c/100*25); p5=p5%1?int(p5)+1:p5; print a[c-p5-1]}')
PERC25=$(echo "$TOTALS" | awk 'BEGIN{c=0} length($0){a[c]=$0;c++}END{p5=(c/100*75); p5=p5%1?int(p5)+1:p5; print a[c-p5-1]}')
STDDEV=$(echo "$TOTALS" | awk '{sum+=$1; sumsq+=$1*$1}END{print sqrt(sumsq/NR - (sum/NR)*2)}')
MEAN=$(echo "$TOTALS" | awk '{a[i++]=$0;s+=$0}END{print s/i}')

echo "25th: $PERC25"
echo "75th: $PERC75"
echo "95th: $PERC95"
echo "Mean: $MEAN"
echo "Deviation: $STDDEV"

STD=$(echo "$STDDEV 1.3" | awk '{print $1*$2}')

# Check series for anomalies
COUNTER=0
         while [  $COUNTER -ne "$TOTAL" ]; do
	     THISTOT=$(echo "$REGSTAT" | jq -r '.data['$COUNTER'] .total' )

		# IF abs(x-mu) > 3*std  THEN  x is outlier
		ABS=$(echo "$THISTOT $MEAN"| awk '{print $1-$2}' | tr -d - )

	     if [ $(echo " $ABS > $STD" | bc) -eq 1 ]; then
		     echo "Outlier:"
		     echo "$REGSTAT" | jq -r '.data['$COUNTER'] | "\(.source_ip) \(.total) \(.method)"'
	     fi
             let COUNTER=COUNTER+1 
         done
