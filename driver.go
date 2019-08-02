package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"regexp"
	"sync"
	"os"
	"time"
	"encoding/json"
	"crypto/md5"
	"errors"
	"strconv"
	"os/exec"
	"io/ioutil"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/Sirupsen/logrus"
	"github.com/docker/go-plugins-helpers/volume"
)

type VolumeInfo struct {
	path 		string
	name_ref	string
	bucket		*oss.Bucket
}

type ALiOssVolumeDriver struct {
	debug	   bool
	driver	   string
	volumes    map[string]*VolumeInfo
	clients	   map[string]*oss.Client
	mutex      *sync.Mutex
	mount	   string
}

type VolumeStorage struct {
	CreatedAt	time.Time
	Driver 		string
	Labels		map[string]string
	Mountpoint	string
	Name		string
	Options		map[string]string
	Scope		string	
}

const ossfsRoot = "/var/lib/ossfs/volumes"
func NewALiOssVolumeDriver(mount string, driver string, ossDef map[string]OssDef, debug bool) volume.Driver {
	clients	:= make(map[string]*oss.Client)
	for name, def := range ossDef {
		client, err := oss.New(def["endpoint"], def["accesskeyid"], def["accesskeysecret"])
		if err != nil {
			fmt.Printf("%c[1;0;31merror: create oss client fail by oss define \"%s\"!%c[0m\n",0x1B, name, 0x1B)
			continue;
		}
		clients[name] = client;
	}
	var d = ALiOssVolumeDriver{
		debug:	    debug,
		driver:	    driver,
		volumes:    make(map[string]*VolumeInfo),
		clients:    clients,
		mutex:      &sync.Mutex{},
		mount:      mount,
	}
	if len(ossDef) <= 0 {
		fmt.Printf("%c[1;0;31merror: has none oss define!%c[0m\n",0x1B, 0x1B)
		return d
	}
	fp := filepath.Join(mount, "*/opts.json")
	tos, _ := ExecuteCmd(fmt.Sprintf("find %s | xargs grep -l '\"Driver\": \"%s\"'", fp, driver), 1, d.debug)
	cfgs := strings.Split(strings.Trim(tos, " "), "\n")
	for _, fn := range cfgs {
		fn = strings.Trim(fn, " ")
		data, err := ioutil.ReadFile(fn)
		if err != nil {
		     	continue
		}
		dv := VolumeStorage{}
		err = json.Unmarshal(data, &dv)
   		if err != nil {
			fmt.Printf("error: %v", err)
		      	continue
		}
		if debug {
			fmt.Printf("restore %s--->name: [%s]	 name_ref: [%s]	bucket: [%s]	path: [%s]\n", fn, dv.Name, dv.Options["name-ref"], dv.Options["bucket"], dv.Options["path"])
		}
		err = d.BuildVolume(dv.Name, dv.Options["name-ref"], dv.Options["bucket"], dv.Options["path"], true)
		if debug {
			if err == nil {
				fmt.Printf("		[ok]\n")
			}else{
				fmt.Printf("		[fail]\n")
			}
		}
	}
	return d
}

func (d ALiOssVolumeDriver) Create(req *volume.CreateRequest) error {
	var name_ref string
	var bucket string
	var path string
	nr, _ := req.Options["name-ref"]
        if nr != "" {
        	name_ref = nr
        }
	bk, _ := req.Options["bucket"]
        if bk != "" {
                bucket = bk
        }
        ph, _ := req.Options["path"]
    	if ph != "" {
    	         path = ph
        }

	return d.BuildVolume(req.Name, name_ref, bucket, path, false)
}

func (d ALiOssVolumeDriver) BuildVolume(name string, name_ref string, bucket string, path string, isLoad bool) error{
	if name == "" {
		var msg = "volume name can't be nil---1"
		fmt.Printf("%c[1;0;31merror: Create volume: %s%c[0m\n",0x1B, msg, 0x1B)
		return errors.New(msg)
	}
	m := strings.Index(name, "[");
	n := strings.Index(name, "]");
	if m != -1 && n != -1 && n > m {
		val := strings.Trim(name[m + 1: n], " ")
		itms := strings.Split(val, ",")
		for _, itm := range itms {
			nvs := strings.Split(strings.Trim(itm, " "), "=")
			if len(nvs) >=2 {
				na := strings.Trim(nvs[0], " ")
				va := strings.Trim(nvs[1], " ")
				if na == "name-ref" {
					name_ref = va
				}else if na == "bucket" {
					bucket = va
				}else if na == "path" {
					path = va
				}
			}
		}
		name = strings.Trim(name[0: m] + name[n + 1: len(name)], " ")
	}
	if name == "" {
		var msg = "volume name can't be nil!---2"
		fmt.Printf("%c[1;0;31merror: Create volume: %s%c[0m\n",0x1B, msg, 0x1B)
		return errors.New(msg)
	}
	if name_ref == "" {
		var msg = "name-ref can't be nil!"
		fmt.Printf("%c[1;0;31merror: Create volume: %s%c[0m\n",0x1B, msg, 0x1B)
		return errors.New(msg)
	}
	if bucket == "" {
		var msg = "oss's bucket can't be nil"
		fmt.Printf("%c[1;0;31merror: Create volume: %s%c[0m\n",0x1B, msg, 0x1B)
		return errors.New(msg)
	}
	if path == "" {
		path = "/"
	}
	client, ok := d.clients[name_ref]
	if client == nil || !ok {
		var msg = fmt.Sprintf("oss client of %s not exists!", name_ref)
		fmt.Printf("%c[1;0;31merror: Create volume: %s%c[0m\n",0x1B, msg, 0x1B)
		return errors.New(msg)
	}
	ok, err := client.IsBucketExist(bucket)
	if !ok {
		var msg = fmt.Sprintf("the bucket of %s not exists in client %s!", bucket, name_ref)
		fmt.Printf("%c[1;0;31merror:  Create volume: %s%c[0m\n", 0x1B, msg, 0x1B)
		if err != nil {
			panic(err)
		}
		return errors.New(msg)
	}
	
	bkt, err := client.Bucket(bucket)
	if bkt == nil || err != nil {
		var msg = fmt.Sprintf("the bucket of %s not exists in client %s!", bucket, name_ref)
                fmt.Printf("%c[1;0;31merror:  Create volume: %s%c[0m\n",0x1B, msg, 0x1B)
                panic(err)
                return errors.New(msg)
        }
	reg := regexp.MustCompile(`[/\\]+`)
	path = reg.ReplaceAllString(path, string(os.PathSeparator))
	if path[0] == os.PathSeparator {
		path = path[1: len(path)]
	}
	if path[len(path)-1] != os.PathSeparator {
		path = path + string(os.PathSeparator)
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	if _, ok := d.volumes[name]; ok {
                return nil
        }
	ok, err = bkt.IsObjectExist(path)
	if !ok {
		err := bkt.PutObject(path, strings.NewReader(""))
		if err != nil {
			var msg = fmt.Sprintf("create path of %s fail in bucket of %s on client of %s!", path, bucket, name_ref)
			fmt.Printf("%c[1;0;31merror:  Create volume: %s%c[0m\n",0x1B, msg, 0x1B)
    	                panic(err)
    	                return errors.New(msg)
		}
	}
	af := d.mountpoint(name, false)
	if !IsExist(af) {
		 os.MkdirAll(af, os.ModePerm)
	}
	go func(){
       		tos, _ := ExecuteCmd("docker volume inspect " + name, 1, d.debug);
    	        if !(strings.Contains(tos, "[") && strings.Contains(tos, "]")) {
			return
            	}
		fp := filepath.Join(af, "opts.json")
		f, err := os.Create(fp)
		if err != nil{
     			fmt.Println(err.Error())
		        return
		}
		defer func(){
			f.Close()
			ExecuteCmd("chmod 110 " + fp, 1, d.debug)
		}()
		umx := fmt.Sprintf("\"Options\": {\n            \"bucket\": \"%s\",\n            \"name-ref\": \"%s\",\n            \"path\": \"%s\"\n        }", bucket, name_ref, path)
		otx := strings.Replace(strings.Replace(strings.Replace(tos, "[", "", -1), "]", "", -1), "\"Options\": null", umx, -1)		
    		f.WriteString(otx)
	}()	
	d.volumes[name] = &VolumeInfo{ path: path, bucket: bkt, name_ref: name_ref }
	
	dfm := "Create"
	if isLoad {
		dfm = "Load"
	}
	fmt.Printf("%s the volume of %s point to %s in bucket of %s in client of %s success!\n", dfm, name, path, bucket, name_ref)
	
	return nil
}

func (d ALiOssVolumeDriver) List() (*volume.ListResponse, error) {
	logrus.Info("Volumes list... ")
	var res = &volume.ListResponse{}

	volumes := make([]*volume.Volume, 0)

	for name, _ := range d.volumes {
		volumes = append(volumes, &volume.Volume{
			Name:       name,
			Mountpoint: d.mountpoint(name, true),
		})
	}
	
	res.Volumes = volumes
	return res, nil
}

func (d ALiOssVolumeDriver) Get(r *volume.GetRequest) (*volume.GetResponse,error) {
	name := r.Name
	m := strings.Index(name, "[");
        n := strings.Index(name, "]");
      	if m != -1 && n != -1 && n > m {
        	name = strings.Trim(name[0: m] + name[n + 1: len(name)], " ")
	}
	logrus.Infof("Get volume: %s", name)
	var res = &volume.GetResponse{}

	if _, ok := d.volumes[name]; ok {
		res.Volume = &volume.Volume{
			Name:       name,
			Mountpoint: d.mountpoint(name, true),
		}
		return res, nil
	}
	return &volume.GetResponse{}, errors.New(name + " not exists")
}

func (d ALiOssVolumeDriver) Remove(r *volume.RemoveRequest) error {
	logrus.Info("Remove volume ", r.Name)
	d.mutex.Lock()
	defer d.mutex.Unlock()

	vi, ok := d.volumes[r.Name]
	if !ok {
		return errors.New(r.Name + " not exists")
	}
	go func(){
		tos, _ := ExecuteCmd(fmt.Sprintf("find %s/*/opts.json | xargs grep -El '\"Driver\": \"%s\" | \"name-ref\": \"%s\"' | wc -l", d.mount, d.driver,vi.name_ref), 1, d.debug)
		reg := regexp.MustCompile(`\D+`)
		ufx := reg.ReplaceAllString(tos, "")
		cnt, err := strconv.ParseInt(ufx, 10, 32)
		if err != nil {
			panic(err)
			return
		}
                ExecuteCmd("rm -rf " + d.mountpoint(r.Name, false), 2, d.debug)
		if cnt > 1 {
			return
		}
		if d.debug {
			fmt.Printf("current is the last volume of the mount %s, now ready to unmount %s!\n", vi.name_ref, vi.name_ref)
		}
                bkn := vi.bucket.BucketName
                pkp := filepath.Join(ossfsRoot, ToMd5(bkn))
                
                tos, _ = ExecuteCmd("mountpoint " + pkp, 3, d.debug)
                if strings.Contains(tos, "is a mountpoint") {
                       	_, err := ExecuteCmd("fusermount -u " + pkp, 4, d.debug)
			if err == nil {
				os.RemoveAll(pkp)				
			}else{
				fmt.Printf("%v", err)
			}
                }else{
			 os.RemoveAll(pkp)
		}
		fmt.Printf("volume remove success!")
	}()
	delete(d.volumes, r.Name)
	return nil
}

func (d ALiOssVolumeDriver) Path(r *volume.PathRequest) (*volume.PathResponse,error) {
	logrus.Info("Get volume path ", r.Name)

	var res = &volume.PathResponse{}

	if _, ok := d.volumes[r.Name]; ok {
		res.Mountpoint = d.mountpoint(r.Name, true)
		return res, nil
	}
	return &volume.PathResponse{}, errors.New(r.Name + " not exists")
}

func (d ALiOssVolumeDriver) Mount(r *volume.MountRequest) (*volume.MountResponse,error) {
	logrus.Info("Mount volume ", r.Name)
	vi, ok := d.volumes[r.Name]
	if !ok {
		return &volume.MountResponse{},errors.New(r.Name + " not exists")
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	bkn := vi.bucket.BucketName
        pkp := filepath.Join(ossfsRoot, ToMd5(bkn))

	tos, _ := ExecuteCmd("mountpoint " + pkp, 1, d.debug);
	if !strings.Contains(tos, "is a mountpoint") {
		if strings.Contains(tos, "is not a mountpoint"){
			os.RemoveAll(pkp)
		}
		cfg := vi.bucket.Client.Config
		os.Setenv("OSSACCESSKEYID", cfg.AccessKeyID)
		os.Setenv("OSSSECRETACCESSKEY", cfg.AccessKeySecret)
		fmt.Printf("bucket: [%s]  ep: [%s]   aki: [%s]  aks: [%s]\n", vi.bucket.BucketName,  cfg.Endpoint, os.Getenv("OSSACCESSKEYID"), os.Getenv("OSSSECRETACCESSKEY"))
		os.MkdirAll(pkp, os.ModePerm)
		_, err := ExecuteCmd(fmt.Sprintf("ossfs %s %s -ourl=%s", bkn, pkp, cfg.Endpoint), 2, d.debug)
		if err != nil {
			return nil, err
		}
	}
	
	aph := filepath.Join(pkp, vi.path)
	if !IsExist(aph) {
		return nil, errors.New("aim path " + aph + " is not exists!")
	}
	
	af := d.mountpoint(r.Name, true)
	tos, _ = ExecuteCmd("ls -l --color=auto " + af, 2, d.debug);
        if strings.Contains(tos, af + " ->") {
		ExecuteCmd("rm -rf " + af, 3, d.debug)
	}else if strings.Contains(tos, "No such file or directory"){
		os.MkdirAll(d.mount, os.ModePerm)
		os.RemoveAll(af)
	}
        _, err := ExecuteCmd(fmt.Sprintf("ln -s %s %s", aph, af), 4, d.debug)
        if err != nil {
		panic(err)
		return nil, err
        }
	var res = &volume.MountResponse{}
	res.Mountpoint = af
	return res, nil
}

func (d ALiOssVolumeDriver) Unmount(r *volume.UnmountRequest) error {
	logrus.Info("Unmount ", r.Name)
	_, ok := d.volumes[r.Name]
	if !ok {
		return errors.New(r.Name + " not exists")
	}
	return nil	
}

func (d ALiOssVolumeDriver) Capabilities() *volume.CapabilitiesResponse {
	logrus.Info("Capabilities. ")
	return &volume.CapabilitiesResponse{ Capabilities: volume.Capability{Scope: "global"} }
}

func (d ALiOssVolumeDriver)mountpoint(name string, isData bool) string {
	sm := filepath.Join(d.mount, name)
	if isData {
		return filepath.Join(sm, "_data")
	}else{
		return sm
	}
}

func ToMd5(str string) string {
    data := []byte(str)
    has := md5.Sum(data)
    md5str := fmt.Sprintf("%x", has)
    return md5str
}

func IsExist(f string) bool {
    _, err := os.Stat(f)
    return err == nil || os.IsExist(err)
}

func ExecuteCmd(cmd string, index int, debug bool) (string, error){
	if index != -1 && debug {
		fmt.Printf("	excute %d : %s\n", index, cmd)
	}
	out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
        tos := string(out)
	if index != -1 && debug {
		prev := "               ---> "
	   	fmt.Printf(prev + strings.Replace(tos, "\n", "\n" + prev, -1) + "\n")
	}
	return tos, err
}
