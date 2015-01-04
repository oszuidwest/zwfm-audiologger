# Configuratie includen
source config.sh

# Bestanden ouder dan MAXAGE vinden
find $_OUTPUTLOCATION* -mtime +$_MAXAGE -exec rm {} \;

# Audiologger starten
wget $_STREAM --output-document=$_OUTPUTLOCATION/$_DATE.mp3 --user-agent="Audiologger ZWFM"

# TODO: Manier vinden om ieder uur af te breken (cronjob)
# TODO: Beetje logging?
# TODO: Wat als connectie halverwege faalt?
# TODO: Wat als dit al op xx:xx:59 sec gestart wordt, verkeerd uur in de naam dan...