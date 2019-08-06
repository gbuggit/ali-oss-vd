#  Docker Volume Plugin for Aliyun Ossfs

## Overview
ali-oss-vd is a docker volume plugin for aliyun ossfs.It's written in Go and can be deployed as a standalone binary.

## Version history

## Prerequisite
The volume plugin driver relys on the ossfs driver of aliyun, you install ossfs as follows:
```bash
wget http://docs-aliyun.cn-hangzhou.oss.aliyun-inc.com/assets/attach/32196/cn_zh/1496671412523/ossfs_1.80.2_centos7.0_x86_64.rpm
yum localinstall ossfs_1.80.2_centos7.0_x86_64.rpm
```
that's enough for this docker volume plugin driver! if you want to use it independently in the host, you need to do this:
```bash
echo <BUCKET-NAME>:<AccessKeyId>:<AccessKeySecret> > /etc/passwd-ossfs
chmod 640 /etc/passwd-ossfs
ossfs <PATH IN BUCKET] <MOUNTPOINT> -ourl=<EndPoint>		--->mount
fusermount -u <MOUNTPOINT>					--->ummount
```
## Installation
Ensure you have Docker 1.8 or above installed.
```bash
wget -P /usr/bin https://github.com/gbuggit/ali-oss-vd/releases/download/v0.1-alpha/ali-oss-vd
```
## Run the driver on the command line
* Use configuration file
```bash
mkdir /var/lib/ossfs
vi /var/lib/ossfs/def.conf
----------------------------------------------------------------------
[M1]			# You can name ossfs connection as M1
endpoint=AAA1		# ossfs EndPoint
accesskeyid=AAA2	# ossfs AccessKeyId
accesskeysecret=AAA3	# ossfs AccessKeySecret
    	
[M2]
endpoint=BBB1
accesskeyid=BBB2
accesskeysecret=BBB3
----------------------------------------------------------------------
ali-oss-vd -config /var/lib/ossfs/def.conf
```
* Use a docker swarm config variant
The host of running this driver must be a docker swarm manager node. Create a config variant named conf.oss.driver.volume with the swarm manage tool in host, the content is the same with config file above. And then:
```bash
ali-oss-vd -config conf.oss.driver.volume
```
## How to use it?
* first create a docker volume with this driver in host, then use the volume in docker container:
```bash
docker volume create --name <VOLUME-NAME> -d ali-oss-vd -o name-ref=<The name defined in config, such as M1> -o bucket=<BUCKET-NAME> -o path=<PATH IN BUCKET>
docker run -it --name <CONTAINER-NAME> -v <VOLUME-NAME>:/data --volume-driver=ali-oss-vd alpine
--->cd data
--->ls
```
* Use this driver directly to create this volume driver when starting containers:
```bash
docker run -it --name <CONTAINER-NAME> -v <VOLUME-NAME>[name-ref=<The name defined in config, such as M1>,bucket=<BUCKET-NAME>,path=<PATH IN BUCKET>]:/data --volume-driver=ali-oss-vd alpine
```
## Build this driver
The operating system of this builded driver  is:
```bash
cat /etc/redhat-release
--->CentOS Linux release 7.6.1810 (Core)
```
* Preparing golang environment
```bash
yum install golang
go version
--->go version go1.11.5 linux/amd64
mkdir /root/go
```
* Preparing dependent packages
```bash
cd /root/go
mkdir src
cd src

mkdir -p github.com/aliyun
git clone https://github.com/aliyun/aliyun-oss-go-sdk.git ./github.com/aliyun/aliyun-oss-go-sdk

mkdir -p github.com/Sirupsen
git clone https://github.com/Sirupsen/logrus.git ./github.com/Sirupsen/logrus

mkdir -p github.com/docker
git clone https://github.com/docker/go-plugins-helpers.git ./github.com/docker/go-plugins-helpers

git clone https://github.com/docker/go-connections.git ./github.com/docker/go-connections

git clone https://github.com/moby/moby.git ./github.com/docker/docker

mkdir -p github.com/coreos
git clone https://github.com/coreos/go-systemd.git ./github.com/coreos/go-systemd

mkdir -p github.com/go-ini
git clone https://github.com/go-ini/ini.git ./github.com/go-ini/ini

mkdir -p golang.org/x
git clone https://github.com/golang/time.git ./golang.org/x/time

git clone https://github.com/golang/sys.git ./golang.org/x/sys
```
* Preparing and building this driver code
```bash
cd /root/go
git clone https://github.com/gbuggit/ali-oss-vd.git
cd ali-oss-vd
go build -ldflags '-w -s'
```
* Preparing the executable file compression environment
```bash
yum install upx
upx version
```
### Compressing executable file ali-oss-vd
```bash
cd /root/go/ali-oss-vd
upx -9 -qvfk ali-oss-vd
```
You can see that the file is much smaller.

