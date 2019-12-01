package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	DIRECTORY = "cache"
)

var (
	ExistDir  *existMap
	ExistFile *existMap
	FileMap   *fileMap
)

func init() {
	ExistDir = &existMap{
		Index: make(map[string]bool, 1024),
	}
	ExistDir = &existMap{
		Index: make(map[string]bool, 1024),
	}
	FileMap = &fileMap{
		Index: make(map[string]*cacheBody, 100),
	}
}

func main() {
	go save()
	http.HandleFunc("/", proxy)
	err := http.ListenAndServe("localhost:9001", nil)
	if err != nil {
		log.Fatal(err)
	}
}

func proxy(w http.ResponseWriter, r *http.Request) {
	log.Println(r.RequestURI)
	r.Header.Del("Proxy-Connection")
	r.Header.Del("proxy-connection")

	if r.Method == http.MethodPost {
		doPost(w, r)
		return
	}

	cb := cacheProcess(r)
	if cb == nil || len(cb.Body) == 0 {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	//if Body.Status
	for k, v := range cb.Header {
		w.Header().Set(k, v)
	}
	w.Write(cb.Body)
}

func checkExist(fname string) bool {
	_, err := os.Stat(fname)
	if os.IsNotExist(err) {
		return false
	}
	return true
}

func cacheProcess(r *http.Request) *cacheBody {
	u, err := url.Parse(r.RequestURI)
	if err != nil {
		log.Println(err)
		return nil
	}
	log.Println(u.Path)

	cache := parseCache(u.Path)
	if cache == nil {
		return nil
	}

	var cb *cacheBody
	FileMap.RLock()
	if _, ok := FileMap.Index[cache.String()]; !ok {
		FileMap.RUnlock()
		FileMap.Lock()
		if _, ok := FileMap.Index[cache.String()]; !ok {
			FileMap.Index[cache.String()] = &cacheBody{
				Status: false,
				Header: make(map[string]string, 10),
			}
		}
		cb = FileMap.Index[cache.String()]
		FileMap.Unlock()
	} else {
		cb = FileMap.Index[cache.String()]
		FileMap.RUnlock()
	}
	cb.RLock()
	if cb.Status {
		cb.RUnlock()
		return cb
	}
	cb.RUnlock()
	cb.Lock()
	defer cb.Unlock()

	if cb.Status {
		return cb
	}
	if checkExist(cache.String()) {
		f, err := os.Open(cache.String())
		if err != nil {
			log.Println(err)
			goto DOWNLOAD
		}
		b, err := ioutil.ReadAll(f)
		if err != nil {
			log.Println(err)
			goto DOWNLOAD
		}
		cb.Unmarshal(b)
		log.Printf("load %s ok", cache.String())
		return cb
	}
DOWNLOAD:
	b, h := getBody(r.RequestURI, r.Header.Clone())
	for k, v := range *h {
		if len(v) > 0 {
			cb.Header[k] = v[0]
		}
	}
	cb.Status = true
	cb.Body = b
	return cb
}

type cache struct {
	dir      string
	filename string
}

func (c *cache) String() string {
	return c.dir + "/" + c.filename
}

type cacheBody struct {
	Status bool              `json:"status"`
	Body   []byte            `json:"body"`
	Header map[string]string `json:"header"`
	sync.RWMutex
}

func (cb *cacheBody) Marshal() []byte {
	cb.RLock()
	defer cb.RUnlock()
	b, _ := json.Marshal(cb)
	return b
}

func (cb *cacheBody) Unmarshal(b []byte) {
	err := json.Unmarshal(b, cb)
	if err != nil {
		log.Println("unmarshal file failed")
	}
}

type existMap struct {
	Index map[string]bool
	sync.RWMutex
}

type fileMap struct {
	Index map[string]*cacheBody
	sync.RWMutex
}

func parseCache(url string) *cache {
	if url == "" {
		return nil
	}

	if url[0] == '/' {
		if len(url) == 1 {
			return nil
		}
		url = url[1:]
	}
	s := strings.Split(url, "/")
	if len(s) == 1 {
		return &cache{
			dir:      "",
			filename: url,
		}
	}
	return &cache{
		dir:      strings.Join(s[:len(s)-1], "/"),
		filename: s[len(s)-1],
	}
}

func checkAndMkdir(dir string) bool {
	ExistDir.RLock()
	if _, ok := ExistDir.Index[dir]; ok {
		ExistDir.RUnlock()
		return true
	}
	ExistDir.RUnlock()
	ExistDir.Lock()
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		log.Println(err)
		ExistDir.Unlock()
		return false
	}
	ExistDir.Index[dir] = true
	ExistDir.Unlock()
	return true
}

func getBody(url string, header http.Header) ([]byte, *http.Header) {
	cli := http.Client{}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header = header
	resp, err := cli.Do(req)
	if err != nil {
		log.Println(err)
		return nil, nil
	}
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
		return nil, nil
	}
	return b, &resp.Header

}

func doPost(w http.ResponseWriter, r *http.Request) {
	cli := http.Client{}
	req, _ := http.NewRequest(http.MethodPost, r.RequestURI, r.Body)
	req.Header = r.Header

	resp, err := cli.Do(req)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	for k, v := range resp.Header {
		if len(v) > 0 {
			w.Header().Set(k, v[0])
		}
	}
	w.Write(b)
}

func save() {
	tick := time.Tick(10 * time.Second)
	for {
		select {
		case <-tick:
			FileMap.RLock()
			for fileName, cacheBody := range FileMap.Index {
				saveFile(fileName, cacheBody)
			}
			FileMap.RUnlock()
		}
	}
}

func saveFile(fileName string, cacheBody *cacheBody) {
	if checkExist(fileName) {
		return
	}
	cache := parseCache(fileName)
	if !checkAndMkdir(cache.dir) {
		return
	}
	f, err := os.Create(fileName)
	if err != nil {
		log.Println(err)
		return
	}
	f.Write(cacheBody.Marshal())
	f.Close()
	log.Printf("save %s ok\n", fileName)
}
