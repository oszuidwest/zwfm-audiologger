#!/usr/bin/env bash

## Configuratie 
STREAM='http://icecast.zuidwestfm.nl/zuidwest.mp3'
SCRIPTROOT=/root/audiologger
LOGDIR=/usr/share/icecast/web/audiologger
#poept de datum en uur uit. Hier bijv: "01-04-2015_20u" (via: http://www.thegeekstuff.com/2013/05/date-command-examples/)
TIMESTAMP=$(date +"%m-%d-%Y_%Hu")

# Hoeveel dagen moet de audio bewaard blijven
KEEP=14