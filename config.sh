#!/usr/bin/env bash

_OUTPUTLOCATION=/var/www/html #in welke map moeten de bestanden komen?
_STREAM=http://icecast.zuidwestfm.nl/zuidwest.mp3 #link naar icecast- of shoutcast-stream
_MAXAGE=14 #in dagen
_DATE=date +"%m-%d-%Y_%Hu" #poept de datum en uur uit. Bijvoorbeeld: "01-04-2015_20u" (via: http://www.thegeekstuff.com/2013/05/date-command-examples/)