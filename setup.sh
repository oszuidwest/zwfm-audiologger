#initial setup, centos 6

#Timezone goed zetten (werkt mogelijk niet op OpenVZ)
cp /usr/share/zoneinfo/Europe/Amsterdam /etc/localtime

#Yummin' some stuff
yum update -y
yum install ntpdate
yum install httpd 
yum install wget 

#Datum en tijd goedzetten
ntpdate ntp.xs4all.nl