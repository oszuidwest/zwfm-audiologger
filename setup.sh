### Initial setup, CentOS 6

### INSTALLEER ZELF ICECAST OF HTTPD
 
## Configuratie includen
source config.sh

## Timezone goed zetten (werkt mogelijk niet op OpenVZ)
cp /usr/share/zoneinfo/Europe/Amsterdam /etc/localtime

## Yummin' some stuff
yum update -y
yum install ntpdate -y
yum install wget -y

## Datum en tijd goedzetten
ntpdate ntp.xs4all.nl

## Cronjob zetten
touch /etc/cron.hourly/0audiologger
echo "sh $SCRIPTROOT/audiologger.sh" >> /etc/cron.hourly/0audiologger