#!/bin/bash

HOMER_TMP_DIR="/var/www/html/webhomer/tmp"
LOG_FILE="/var/log/webhomer_cleanup.log"

DAYS_TO_KEEP=7

sdate=`date '+%Y-%m-%d %H:%M:%S'`
echo -e "\n[$sdate] - File cleanup process started" >> $LOG_FILE

# Find and delete old backup files
fdate=`date '+%Y-%m-%d %H:%M:%S'`
echo "[$fdate] - Finding and deleting files under homer tmp directory older than $DAYS_TO_KEEP days old" >> $LOG_FILE
cleanup_file_cnt=`find $HOMER_TMP_DIR -type f -mtime +$DAYS_TO_KEEP | wc -l`
cdate=`date '+%Y-%m-%d %H:%M:%S'`
echo -e "[$cdate] - $cleanup_file_cnt file(s) found to be removed under $HOMER_TMP_DIR"  >> $LOG_FILE

for cleanupfile in `find $HOMER_TMP_DIR -type f -mtime +$DAYS_TO_KEEP | xargs -i basename \{\}`; do
        rdate=`date '+%Y-%m-%d %H:%M:%S'`
        echo -e "[$rdate] - Deleting file: $cleanupfile"  >> $LOG_FILE
        rm -f $HOMER_TMP_DIR/$cleanupfile 2>/dev/null
done

edate=`date '+%Y-%m-%d %H:%M:%S'`
echo -e "[$edate] - File cleanup process finished" >> $LOG_FILE

