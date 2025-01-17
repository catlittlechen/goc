/*
 Copyright 2020 Qiniu Cloud (qiniu.com)

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package cover

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"text/template"
)

// InjectCountersHandlers generate a file _cover_http_apis.go besides the main.go file
func InjectCountersHandlers(tc TestCover, dest string) error {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	if err := coverMainTmpl.Execute(f, tc); err != nil {
		return err
	}
	return nil
}

var coverMainTmpl = template.Must(template.New("coverMain").Parse(coverMain))

const coverMain = `
// Code generated by goc system. DO NOT EDIT.

package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	_log "log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"

	_cover {{.GlobalCoverVarImportPath | printf "%q"}}

)

var (
	gGocCoverName = map[string]struct{}{}
	gGocCoverFileContent = map[string]string{}
)

func init() {
	{{range $i, $pkgCover := .DepsCover}}
	{{range $file, $cover := $pkgCover.Vars}}
	gGocCoverFileContent[{{printf "%q" $cover.File}}] = {{printf "%q" $cover.Content}}
	{{end}}
	{{end}}

	{{range $file, $cover := .MainPkgCover.Vars}}
	gGocCoverFileContent[{{printf "%q" $cover.File}}] = {{printf "%q" $cover.Content}}
	{{end}}

	go registerHandlersGoc()
}

func loadValuesGoc() (map[string][]uint32, map[string][]testing.CoverBlock) {
	var (
		coverCounters = make(map[string][]uint32)
		coverBlocks   = make(map[string][]testing.CoverBlock)
	)

	{{range $i, $pkgCover := .DepsCover}}
	{{range $file, $cover := $pkgCover.Vars}}
	loadFileCoverGoc(coverCounters, coverBlocks, {{printf "%q" $cover.File}}, _cover.{{$cover.Var}}.Count[:], _cover.{{$cover.Var}}.Pos[:], _cover.{{$cover.Var}}.NumStmt[:])
	{{end}}
	{{end}}

	{{range $file, $cover := .MainPkgCover.Vars}}
	loadFileCoverGoc(coverCounters, coverBlocks, {{printf "%q" $cover.File}}, _cover.{{$cover.Var}}.Count[:], _cover.{{$cover.Var}}.Pos[:], _cover.{{$cover.Var}}.NumStmt[:])
	{{end}}

	return coverCounters, coverBlocks
}

func loadFileCoverGoc(coverCounters map[string][]uint32, coverBlocks map[string][]testing.CoverBlock, fileName string, counter []uint32, pos []uint32, numStmts []uint16) {
	if 3*len(counter) != len(pos) || len(counter) != len(numStmts) {
		panic("coverage: mismatched sizes")
	}
	if coverCounters[fileName] != nil {
		// Already registered.
		return
	}
	coverCounters[fileName] = counter
	block := make([]testing.CoverBlock, len(counter))
	for i := range counter {
		block[i] = testing.CoverBlock{
			Line0: pos[3*i+0],
			Col0:  uint16(pos[3*i+2]),
			Line1: pos[3*i+1],
			Col1:  uint16(pos[3*i+2] >> 16),
			Stmts: numStmts[i],
		}
	}
	coverBlocks[fileName] = block
}

func clearValuesGoc() {

	{{range $i, $pkgCover := .DepsCover}}
	{{range $file, $cover := $pkgCover.Vars}}
	clearFileCoverGoc(_cover.{{$cover.Var}}.Count[:])
	{{end}}
	{{end}}

	{{range $file, $cover := .MainPkgCover.Vars}}
	clearFileCoverGoc(_cover.{{$cover.Var}}.Count[:])
	{{end}}

}

func clearFileCoverGoc(counter []uint32) {
	for i := range counter {
		counter[i] = 0
	}
}

func registerHandlersGoc() {
	{{if .Singleton}}
	ln, _, err := listenGoc()
	{{else}}
	ln, host, err := listenGoc()
	{{end}}
	if err != nil {
		_log.Fatalf("listenGoc failed, err:%v", err)
	}
	{{if not .Singleton}}
	profileAddr := "http://" + host
	if resp, err := registerSelfGoc(profileAddr); err != nil {
		_log.Fatalf("register address %v failed, err: %v, response: %v", profileAddr, err, string(resp))
	}

	fn := func() {
		var (
			err          error
			profileAddrs []string
			addresses    []string
		)
		if addresses, err = getAllHostsGoc(ln); err != nil {
			_log.Fatalf("get all host failed, err: %v", err)
			return
		}
		for _, addr := range addresses {
			profileAddrs = append(profileAddrs, "http://"+addr)
		}
		deregisterSelfGoc(profileAddrs)
	}
	go watchSignalGoc(fn)
	{{end}}

	mux := http.NewServeMux()
	// Coverage reports the current code coverage as a fraction in the range [0, 1].
	// If coverage is not enabled, Coverage returns 0.
	mux.HandleFunc("/v1/cover/coverage", func(w http.ResponseWriter, r *http.Request) {
		counters, _ := loadValuesGoc()
		var n, d int64
		for name, counter := range counters {
			if len(gGocCoverName) != 0 {
				if _, ok := gGocCoverName[name]; !ok {
					continue
				}
			}
			for i := range counter {
				if atomic.LoadUint32(&counter[i]) > 0 {
					n++
				}
				d++
			}
		}
		if d == 0 {
			fmt.Fprint(w, 0)
			return
		}
		fmt.Fprintf(w, "%f", float64(n)/float64(d))
	})

	// coverprofile reports a coverage profile with the coverage percentage
	mux.HandleFunc("/v1/cover/profile", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "mode: {{.Mode}}\n")
		counters, blocks := loadValuesGoc()
		var active, total int64
		var count uint32
		for name, counts := range counters {
			if len(gGocCoverName) != 0 {
				if _, ok := gGocCoverName[name]; !ok {
					continue
				}
			}
			block := blocks[name]
			for i := range counts {
				stmts := int64(block[i].Stmts)
				total += stmts
				count = atomic.LoadUint32(&counts[i]) // For -mode=atomic.
				if count > 0 {
					active += stmts
				}
				_, err := fmt.Fprintf(w, "%s:%d.%d,%d.%d %d %d\n", name,
					block[i].Line0, block[i].Col0,
					block[i].Line1, block[i].Col1,
					stmts,
					count)
				if err != nil {
					fmt.Fprintf(w, "invalid block format, err: %v", err)
					return
				}
			}
		}
	})

	mux.HandleFunc("/v1/cover/clear", func(w http.ResponseWriter, r *http.Request) {
		clearValuesGoc()
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "clear call successfully")
	})

	mux.HandleFunc("/v1/cover/file/set", func(w http.ResponseWriter, r *http.Request) {
		counters, _ := loadValuesGoc()
		q := r.URL.Query()

		nameList := q["name"]
		if len(nameList) == 0 {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, "miss name param")
			return
		}
		name := nameList[0]

		if _, ok := counters[name]; ok {
			gGocCoverName[name] = struct{}{}
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, "set file successfully")
		} else {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, "file not found. set file fail")
		}
	})

	mux.HandleFunc("/v1/cover/file/unset", func(w http.ResponseWriter, r *http.Request) {
		counters, _ := loadValuesGoc()
		q := r.URL.Query()

		nameList := q["name"]
		if len(nameList) == 0 {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, "miss name param")
			return
		}
		name := nameList[0]

		if _, ok := counters[name]; ok {
			delete(gGocCoverName, name)
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, "unset file successfully")
		} else {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, "file not found. unset file fail")
		}
	})

	mux.HandleFunc("/v1/cover/file/clear", func(w http.ResponseWriter, r *http.Request) {
		gGocCoverName = map[string]struct{}{}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "set file successfully")
	})

	mux.HandleFunc("/v1/cover/content/get", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		nameList := q["name"]
		if len(nameList) == 0 {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintln(w, "miss name param")
			return
		}
		name := nameList[0]

		w.WriteHeader(http.StatusOK)
		if content, ok := gGocCoverFileContent[name]; ok {
			data, err := base64.StdEncoding.DecodeString(content)
			if err != nil {
				fmt.Fprintln(w, "file content wrong. ")
				return
			}
			fmt.Fprintln(w, string(data))
		} else {
			fmt.Fprintln(w, "file not found. ")
		}
	})

	_log.Fatal(http.Serve(ln, mux))
}

func registerSelfGoc(address string) ([]byte, error) {
	customServiceName, ok := os.LookupEnv("GOC_SERVICE_NAME")
	var selfName string
	if ok {
		selfName = customServiceName
	} else {
		selfName = filepath.Base(os.Args[0])
	}
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/v1/cover/register?name=%s&address=%s", {{.Center | printf "%q"}}, selfName, address), nil)
	if err != nil {
		_log.Fatalf("http.NewRequest failed: %v", err)
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil && isNetworkErrorGoc(err) {
		_log.Printf("[goc][WARN]error occurred:%v, try again", err)
		resp, err = http.DefaultClient.Do(req)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to register into coverage center, err:%v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body, err:%v", err)
	}

	if resp.StatusCode != 200 {
		err = fmt.Errorf("failed to register into coverage center, response code %d", resp.StatusCode)
	}

	return body, err
}

func deregisterSelfGoc(address []string) ([]byte, error) {
	param := map[string]interface{}{
		"address": address,
	}
	jsonBody, err := json.Marshal(param)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/v1/cover/remove", {{.Center | printf "%q"}}), bytes.NewReader(jsonBody))
	if err != nil {
		_log.Fatalf("http.NewRequest failed: %v", err)
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil && isNetworkErrorGoc(err) {
		_log.Printf("[goc][WARN]error occurred:%v, try again", err)
		resp, err = http.DefaultClient.Do(req)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to deregister into coverage center, err:%v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body, err:%v", err)
	}

	if resp.StatusCode != 200 {
		err = fmt.Errorf("failed to deregister into coverage center, response code %d", resp.StatusCode)
	}

	return body, err
}

type CallbackGocFunc func()

func watchSignalGoc(fn CallbackGocFunc) {
	// init signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT)
	for {
		si := <-c
		_log.Printf("get a signal %s", si.String())
		switch si {
		case syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT:
			fn()
			os.Exit(0) // Exit successfully.
		case syscall.SIGHUP:
		default:
			return
		}
	}
}

func isNetworkErrorGoc(err error) bool {
	if err == io.EOF {
		return true
	}
	_, ok := err.(net.Error)
	return ok
}

func listenGoc() (ln net.Listener, host string, err error) {
	agentPort := "{{.AgentPort }}"
	if agentPort != "" {
		if ln, err = net.Listen("tcp4", agentPort); err != nil {
			return
		}
		if host, err = getRealHostGoc(ln); err != nil {
			return
		}
	} else {
		// 获取上次使用的监听地址
		if previousAddr := getPreviousAddrGoc(); previousAddr != "" {
			ss := strings.Split(previousAddr, ":")
			// listenGoc on all network interface
			ln, err = net.Listen("tcp4", ":"+ss[len(ss)-1])
			if err == nil {
				host = previousAddr
				return
			}
		}
		if ln, err = net.Listen("tcp4", ":0"); err != nil {
			return
		}
		if host, err = getRealHostGoc(ln); err != nil {
			return
		}
	}
	go genProfileAddrGoc(host)
	return
}

func getRealHostGoc(ln net.Listener) (host string, err error) {
	adds, err := net.InterfaceAddrs()
	if err != nil {
		return
	}

	var localIPV4 string
	var nonLocalIPV4 string
	for _, addr := range adds {
		if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP.To4() != nil {
			if ipNet.IP.IsLoopback() {
				localIPV4 = ipNet.IP.String()
			} else {
				nonLocalIPV4 = ipNet.IP.String()
			}
		}
	}
	if nonLocalIPV4 != "" {
		host = fmt.Sprintf("%s:%d", nonLocalIPV4, ln.Addr().(*net.TCPAddr).Port)
	} else {
		host = fmt.Sprintf("%s:%d", localIPV4, ln.Addr().(*net.TCPAddr).Port)
	}

	return
}

func getAllHostsGoc(ln net.Listener) (hosts []string, err error) {
	adds, err := net.InterfaceAddrs()
	if err != nil {
		return
	}

	var host string
	for _, addr := range adds {
		if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP.To4() != nil {
			host = fmt.Sprintf("%s:%d", ipNet.IP.String(), ln.Addr().(*net.TCPAddr).Port)
			hosts = append(hosts, host)
		}
	}
	return
}

func getPreviousAddrGoc() string {
	file, err := os.Open(os.Args[0] + "_profile_listen_addr")
	if err != nil {
		return ""
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	addr, _, _ := reader.ReadLine()
	return string(addr)
}

func genProfileAddrGoc(profileAddr string) {
	fn := os.Args[0] + "_profile_listen_addr"
	f, err := os.OpenFile(fn, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		_log.Println(err)
		return
	}
	defer f.Close()

	fmt.Fprintf(f, strings.TrimPrefix(profileAddr, "http://"))
}
`

var coverParentFileTmpl = template.Must(template.New("coverParentFileTmpl").Parse(coverParentFile))

const coverParentFile = `
// Code generated by goc system. DO NOT EDIT.

package {{.}}

`

var coverParentVarsTmpl = template.Must(template.New("coverParentVarsTmpl").Parse(coverParentVars))

const coverParentVars = `

import (

	{{range $i, $pkgCover := .}}
	_cover{{$i}} {{$pkgCover.Package.ImportPath | printf "%q"}}
	{{end}} 

)

{{range $i, $pkgCover := .}}
{{range $v, $cover := $pkgCover.Vars}}
var {{$v}} = &_cover{{$i}}.{{$cover.Var}}
{{end}}
{{end}}
	
`

func InjectCacheCounters(covers map[string][]*PackageCover, cache map[string]*PackageCover) []error {
	var errs []error
	for k, v := range covers {
		if pkg, ok := cache[k]; ok {
			err := checkCacheDir(pkg.Package.Dir)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			_, pkgName := path.Split(k)
			err = injectCache(v, pkgName, fmt.Sprintf("%s/%s", pkg.Package.Dir, pkg.Package.GoFiles[0]))
			if err != nil {
				errs = append(errs, err)
				continue
			}
		}
	}
	return errs
}

// InjectCacheCounters generate a file _cover_http_apis.go besides the main.go file
func injectCache(covers []*PackageCover, pkg, dest string) error {
	f, err := os.Create(dest)
	if err != nil {
		return err
	}

	if err := coverParentFileTmpl.Execute(f, pkg); err != nil {
		return err
	}

	if err := coverParentVarsTmpl.Execute(f, covers); err != nil {
		return err
	}
	return nil
}

func checkCacheDir(p string) error {
	_, err := os.Stat(p)
	if os.IsNotExist(err) {
		err := os.Mkdir(p, 0755)
		if err != nil {
			return err
		}
	}
	return nil
}

func injectGlobalCoverVarFile(ci *CoverInfo, content string) error {
	coverFile, err := os.Create(filepath.Join(ci.Target, ci.GlobalCoverVarImportPath, "cover.go"))
	if err != nil {
		return err
	}
	defer coverFile.Close()

	packageName := "package " + filepath.Base(ci.GlobalCoverVarImportPath) + "\n\n"

	_, err = coverFile.WriteString(packageName)
	if err != nil {
		return err
	}
	_, err = coverFile.WriteString(content)

	return err
}
