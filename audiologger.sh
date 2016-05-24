## Configuratie includen
. /etc/audiologger.conf

## Map maken
if [ !$LOGDIR ];
  then
  /bin/mkdir -p $LOGDIR
fi

## Oude bestanden verwijderen
/bin/find $LOGDIR -type f -mtime +$KEEP -exec rm {} \;

## Vorige uur killen
pids=$(/usr/bin/pgrep -f $STREAMURL)
/bin/kill -9 $pids

## Volgende uur opnemen
/usr/bin/wget --quiet --background --user-agent="Audiologger" -O $LOGDIR/$TIMESTAMP.mp3 $STREAMURL > /dev/null 2>&1

##KLAAR
