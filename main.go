package main

import (
	"log"
	"flag"
	"fmt"
	"os"
	"regexp"
	"context"
	"strings"
	"bufio"
	"github.com/go-ini/ini"
	"github.com/docker/docker/client"
	"github.com/docker/go-plugins-helpers/volume"
)

const defaultVolumeRoot = "/var/lib/docker/volumes" //difference from volume.DefaultDockerRootDirectory
var (
	mount  = defaultVolumeRoot	
	config = flag.String("config", "", "a config file or a config variant name of swarm about alyun oss infomation, for example:\n" +
		"	ali-oss-vd -config /ossfs/def.conf		-->config file\n" +
		"	ali-oss-vd -config conf.oss.driver.volume	-->config variant in docker swarm\n" +
		"whether a file or a variant in docker swarm, the content may be like this:\n" +
		"	-------------------------------------------------------------\n" +
		"	[M1]			# You can name ossfs connection as M1\n" +
		"	endpoint=AAA1		# ossfs EndPoint\n" +
		"	accesskeyid=AAA2	# ossfs AccessKeyId\n" +
		"	accesskeysecret=AAA3	# ossfs AccessKeySecret\n" +
		"\n" +
		"	[M2]\n" +
                "	endpoint=BBB1\n" +
                "	accesskeyid=BBB2\n" +
                "	accesskeysecret=BBB3\n" +
		"	-------------------------------------------------------------\n" +
		"\n\n" +
		"volume use like this:\n" +
		"	docker volume create --name <VOLUME-NAME> -d ali-oss-vd -o name-ref=<M1 OR M2> -o bucket=<BUCKET-NAME> -o path=<PATH IN BUCKET>\n" +
		"	docker run -it --name Test -v <VOLUME-NAME>:/data --volume-driver=ali-oss-vd alpine\n" +
		"or direct like this:\n" +
		"	docker run -it --name Test -v <VOLUME-NAME>[name-ref=<M1 OR M2>,bucket=<BUCKET-NAME>,path=<PATH IN BUCKET>]:/data --volume-driver=ali-oss-vd alpine")
	debug  = flag.Bool("debug", false, "print debug info on console with value true")
)

type OssDef map[string]string
var ossDefItems = []string{"endpoint", "accesskeyid", "accesskeysecret"}
func main() {
	drvn := "ali-oss-vd"	
	var Usage = func() {
		fmt.Printf("Name: " + drvn + "\n")
		fmt.Printf("Describe: a docker volume plugin driver for ossfs of aliyun\n")
		fmt.Printf("Author: gbuggit\n")
		fmt.Printf("Note: this driver rely on ossfs driver of aliyun, install ossfs like this:\n" +
			"	wget http://docs-aliyun.cn-hangzhou.oss.aliyun-inc.com/assets/attach/32196/cn_zh/1496671412523/ossfs_1.80.2_centos7.0_x86_64.rpm\n" +
			"	yum localinstall ossfs_1.80.2_centos7.0_x86_64.rpm\n" +
			"just this is satisfy with current docker volume plugin! if you want to use it independent in host, you need to do this:\n" +
			"	echo <BUCKET-NAME>:<AccessKeyId>:<AccessKeySecret> > /etc/passwd-ossfs\n" +
			"	chmod 640 /etc/passwd-ossfs\n" +
			"	ossfs <PATH IN BUCKET] <MOUNTPOINT> -ourl=<EndPoint>		--->mount\n" +
			"	fusermount -u <MOUNTPOINT>					--->ummount\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", os.Args[0])
		flag.PrintDefaults()
	}
	var Excute = func(dfs map[string]OssDef){
		dbg := false
		if *debug {
			dbg = true
		}		
	    	driver := NewALiOssVolumeDriver(mount, drvn, dfs, dbg)
	        handler := volume.NewHandler(driver)
    	        if err := handler.ServeUnix(drvn, 0); err != nil {
    	                log.Fatalf("Error %v", err)
    	        }
	}
	ots, _ := ExecuteCmd("ossfs --version", 1, *debug)
	if strings.Contains(ots, "command not found"){
		fmt.Printf("%c[1;0;31merror: ossfs not installed!%c[0m\n",0x1B, 0x1B)
		Usage()
		return
	}

	flag.Parse();
	var conf *ini.File
	if len(*config) == 0{
		Usage()
		Excute(nil)
		return
	}
	f, err := os.Stat(*config)
	if err == nil || os.IsExist(err) {
		if f.IsDir() {
			fmt.Printf("%c[1;0;31merror: config \"%s\" must be a file!%c[0m\n",0x1B, *config, 0x1B)
			Usage()
			Excute(nil)
			return
		}
  		fcf, err := ini.Load(*config)
     	        if err != nil {
     	      		log.Fatal(err)
			Usage()
     		        Excute(nil)
			return
     	        }
		conf = fcf
	}else{
		r, _ := regexp.Compile("^[A-Za-z][A-Za-z0-9._-]*$")
		if !r.MatchString(*config) {
			fmt.Printf("%c[1;0;31merror: config \"%s\" is not a exist file and not a swarm config variant name!%c[0m\n",0x1B, *config, 0x1B)
			Usage()
			Excute(nil)
			return
		}
		cnt := 0
LABEL_AGAIN:
		cli, err := client.NewEnvClient()
		if err != nil {
			fmt.Printf("%c[1;0;31merror: config \"%s\" is a swarm config variant name, connect docker host fail!%c[0m\n",0x1B, *config, 0x1B)
			panic(err)
			Usage()
			Excute(nil)
			return
		}
		confspc, _, err := cli.ConfigInspectWithRaw(context.Background(), *config)
		if err != nil {
			fmt.Printf("%c[1;0;31merror: config \"%s\" is a swarm config variant name, get it's content from docker fail:%c[0m\n", 0x1B, *config, 0x1B)
			errTxt := err.Error();
			if strings.Index(errTxt, "is too new. Maximum supported API version is") != -1{
				reg := regexp.MustCompile(`\d+\.\d+$`)
                                ver := reg.FindAllString(errTxt, -1)
				fmt.Printf(errTxt + "\n")
				if cnt == 0 {
					cnt += 1
					fmt.Printf("	auto solve: execute \"export DOCKER_API_VERSION=%s\" first, then try again!\n", ver[0])
					os.Setenv("DOCKER_API_VERSION", ver[0])
                                        goto LABEL_AGAIN
				}
				fmt.Printf("%c[1;0;32mTo Solve: automatically match the current docker API version(export DOCKER_API_VERSION=%s)?[y/n]%c[0m", 0x1B, ver[0], 0x1B)
				scanner := bufio.NewScanner(os.Stdin)
				scanner.Scan()
				if err := scanner.Err(); err != nil {
					fmt.Fprintln(os.Stderr, "error:", err)
					Usage()
					Excute(nil)
					return
				}
				if scanner.Text() == "y" {
					os.Setenv("DOCKER_API_VERSION", ver[0])
					goto LABEL_AGAIN
				}
			}
			Usage()
			Excute(nil)
			return
		}
		bcf, err := ini.Load(confspc.Spec.Data)
    		if err != nil {
                   	log.Fatal(err)
			Usage()
			Excute(nil)
			return
             	}
		conf = bcf
	}
	
	sts := conf.SectionStrings()
	dfs := make(map[string]OssDef)
	for _, sec := range sts {
		item := make(OssDef);
		cnt := 0
		for _, key := range ossDefItems{
			val := conf.Section(sec).Key(key).String();
			if val == "" {
				continue;
			}
			cnt += 1
			item[key] = val;			
		}
		if cnt == 0 {
			continue;
		}
		if cnt < len(ossDefItems) {
			fmt.Printf("oss define \"%s\"...parse...[error]\n", sec)
			continue
		}
		dfs[sec] = item
		fmt.Printf("oss define \"%s\"...parse...[ok]\n", sec);
	}
	if len(dfs) <= 0 {
		fmt.Printf("%c[1;0;32mwarnning: config \"%s\" has't any oss define!%c[0m\n",0x1B, *config, 0x1B)
	}
	Excute(dfs)
}
