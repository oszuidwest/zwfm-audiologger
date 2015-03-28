#!/usr/bin/env bash

## Configuratie 
#
STREAM='http://icecast.zuidwestfm.nl/zuidwest.mp3'
SCRIPTROOT=/logger
TMPDIR=/logger/tmp
LOGDIR=/logger/logs
#poept de datum en uur uit. Hier bijv: "01-04-2015_20u" (via: http://www.thegeekstuff.com/2013/05/date-command-examples/)
TIMESTAMP=$(date +"%m-%d-%Y_%Hu")
USER=logger
#Change the value below to adjust the number of days to keep the logs for (42 is default in line with regulations)
KEEP=14
##