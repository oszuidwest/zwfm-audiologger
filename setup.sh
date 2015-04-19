### Initial setup, CentOS 6

### INSTALLEER ZELF ICECAST OF HTTPD

## Timezone goed zetten (werkt mogelijk niet op OpenVZ)
cp /usr/share/zoneinfo/Europe/Amsterdam /etc/localtime

## Yummin' some stuff
yum update -y
yum install ntpdate -y
yum install wget -y

## Datum en tijd goedzetten
ntpdate ntp.xs4all.nl

## Cronjob zetten
rm -rf /etc/cron.hourly/0audiologger
touch /etc/cron.hourly/0audiologger
echo "sh /root/audiologger/audiologger.sh" >> /etc/cron.hourly/0audiologger
chmod +x /etc/cron.hourly/0audiologger
echo "SHELL=/bin/bash
PATH=/sbin:/bin:/usr/sbin:/usr/bin
MAILTO=root
HOME=/

# For details see man 4 crontabs

# Example of job definition:
# .---------------- minute (0 - 59)
# |  .------------- hour (0 - 23)
# |  |  .---------- day of month (1 - 31)
# |  |  |  .------- month (1 - 12) OR jan,feb,mar,apr ...
# |  |  |  |  .---- day of week (0 - 6) (Sunday=0 or 7) OR sun,mon,tue,wed,thu,fri,sat
# |  |  |  |  |
# *  *  *  *  * user-name command to be executed

00 * * * * root run-parts /etc/cron.hourly
02 4 * * * root run-parts /etc/cron.daily
22 4 * * 0 root run-parts /etc/cron.weekly
42 4 1 * * root run-parts /etc/cron.monthly" > /etc/crontab
