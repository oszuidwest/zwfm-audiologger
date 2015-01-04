# Bestanden ouder dan MAXAGE vinden
find $_OUTPUTLOCATION* -mtime +$_MAXAGE -exec rm {} \;

# Audiologger starten
wget $_STREAM --output-document=$_OUTPUTLOCATION/$_DATE.mp3

