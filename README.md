Simpele audiologger met wget
=================

Over
=====
Dit script neemt audio live streams op.
Gemaakt voor ZuidWest FM

License
=======
GNU GENERAL PUBLIC LICENSE Version 2

This script is provided "as-is" and is supplied without warranty or guarantee.

Installatie - Gebaseerd op CentOS 6.x
============
Voor het installeren *(uitvoeren als root)*
  yum install unzip -y
  wget -O /root/audiologger.zip https://github.com/rmens/audiologger-zwfm/archive/master.zip 
  unzip /root/audiologger.zip
  mkdir /root/audiologger
  mv /root/audiologger-zwfm-master/* /root/audiologger
  rm -rf /root/audiologger-zwfm-master/ /root/audiologger.zip
  chmod +x /root/audiologger/setup.sh /root/audiologger/audiologger.sh* 
  sh /root/audiologger/setup.sh

Plaats de stream-url in config.sh tussen enkele quotes
