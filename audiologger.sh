## Configuratie includen
source config.sh

## Mappen maken
if [ !$TMPDIR ];
  then
  mkdir -p $TMPDIR
  chown $USER $TMPDIR
fi
if [ !$LOGDIR ];
  then
  mkdir -p $LOGDIR
  chown $USER $LOGDIR
fi

## Oude bestanden verwijderen en vorige uur wegschrijven
find $LOGDIR -type f -mtime +$KEEP -exec rm {} \;
find $TMPDIR -type f -mmin +61 -exec mv {} $LOGDIR \;
chown -R $USER $LOGDIR/*


## Vorige uur killen
pids=$(pgrep $STREAMURL)
kill $pids

## Volgende uur opnemen
/usr/bin/wget --background -O $TMPDIR/$TIMESTAMP.mp3 $STREAMURL > /dev/null 2>&1

##KLAAR