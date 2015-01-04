#initial setup, centos 6

yum update -y
yum install ntpdate
ntpdate ntp.xs4all.nl
yum install httpd 
yum install wget 
