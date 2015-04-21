Simpele audiologger met wget
=================

Over
=====
Dit script neemt audio live streams op. Radio-omroepen zijn wettelijk verplicht een 'log' van 14 dagen bij te houden. Wordt ook gebruikt voor uitzending gemist. Als je niet weet wat dit doet, is het waarschinlijk niet voor jou. Gemaakt voor ZuidWest FM.

Licentie
=======
GNU GENERAL PUBLIC LICENSE Version 2

This script is provided "as-is" and is supplied without warranty or guarantee.

Installatie - Gebaseerd op CentOS 6.x
============
Voor het installeren *(uitvoeren als root)*
 ```
yum install unzip wget -y
wget -O /root/audiologger.zip https://github.com/rmens/audiologger-zwfm/archive/master.zip
unzip /root/audiologger.zip
mkdir /root/audiologger 
mv /root/audiologger-zwfm-master/* /root/audiologger
rm -rf /root/audiologger-zwfm-master/ /root/audiologger.zip
chmod +x /root/audiologger/setup.sh /root/audiologger/audiologger.sh* 
sh /root/audiologger/setup.sh
```

Plaats de stream-url in config.sh tussen enkele quotes
