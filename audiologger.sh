## Configuratie includen
source /root/audiologger/config.sh

## Map maken
if [ !$LOGDIR ];
  then
  /bin/mkdir -p $LOGDIR
fi

## Oude bestanden verwijderen
/bin/find $LOGDIR -type f -mtime +$KEEP -exec rm {} \;

## Vorige uur killen
pids=$(/usr/bin/pgrep $STREAMURL)
/bin/kill $pids

## Volgende uur opnemen
/usr/bin/wget --quiet --background -O $LOGDIR/$TIMESTAMP.mp3 $STREAMURL > /dev/null 2>&1

##KLAAR
