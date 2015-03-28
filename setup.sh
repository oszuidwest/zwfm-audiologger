### Initial setup, CentOS 7

## Configuratie includen
source config.sh

## Timezone goed zetten (werkt mogelijk niet op OpenVZ)
cp /usr/share/zoneinfo/Europe/Amsterdam /etc/localtime

## Yummin' some stuff
yum update -y
yum install ntpdate -y
yum install httpd -y
yum install wget -y

## Datum en tijd goedzetten
ntpdate ntp.xs4all.nl

## User toevoegen
adduser $USER