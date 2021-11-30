## Configuratie includen
. /etc/audiologger.conf

## Map maken
if [ !$LOGDIR ];
  then
  /usr/bin/mkdir -p $LOGDIR
fi

## Oude bestanden verwijderen
/usr/bin/find $LOGDIR -type f -mtime +$KEEP -exec /usr/bin/rm {} \;

## Vorige uur killen
pids=$(/usr/bin/pgrep -f $STREAMURL)
/usr/bin/kill -9 $pids

## Volgende uur opnemen
/usr/bin/wget --quiet --background --user-agent="Audiologger ZuidWest (Debian 11)" -O $LOGDIR/$TIMESTAMP.mp3 $STREAMURL > /dev/null 2>&1

##KLAAR
