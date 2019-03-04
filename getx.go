package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/atotto/clipboard"
	"github.com/kardianos/service"
)

var versionG string = "0.93a"

var defaultPortG string = "7468"
var defaultBasePathG string
var defaultConfigFileNameG string = "getx.cfg"
var defaultClipFileNameG string = "clip.txt"

var clipMapG map[string]string = nil
var clipMapLockG sync.Mutex

var maxClipCountG int = 100 + 1
var maxClipSizeG int = 8000

var basePathG string = ""
var dataPathG string = ""
var logFileG string = ""
var serverUrlG = ""

var serviceModeG bool = false

var exit = make(chan struct{})

func logWithTime(formatA string, argsA ...interface{}) {
	if logFileG == "" {
		return
	}

	if !serviceModeG {
		fmt.Printf(fmt.Sprintf("[%v] ", time.Now())+formatA+"\n", argsA...)
		return
	}

	appendStringToFile(fmt.Sprintf(fmt.Sprintf("[%v] ", time.Now())+formatA+"\n", argsA...), logFileG)
}

func fileExists(fileNameA string) bool {
	_, err := os.Stat(fileNameA)
	return err == nil || os.IsExist(err)
}

func isDirectory(dirNameA string) bool {
	f, err := os.Open(dirNameA)
	if err != nil {
		return false
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return false
	}

	if mode := fi.Mode(); mode.IsDir() {
		return true
	} else {
		return false
	}
}

func ensureMakeDirs(pathA string) string {
	if !fileExists(pathA) {
		os.MkdirAll(pathA, 0777)
		return ""
	} else {
		if isDirectory(pathA) {
			return ""
		} else {
			return "a file with same name exists"
		}
	}
}

func loadString(fileNameA string) (string, bool) {
	if !fileExists(fileNameA) {
		return "file not exists", false
	}

	fileT, err := os.Open(fileNameA)
	if err != nil {
		return err.Error(), false
	}

	defer fileT.Close()

	fileContentT, err := ioutil.ReadAll(fileT)
	if err != nil {
		return err.Error(), false
	}

	return string(fileContentT), true
}

func loadStringList(fileNameA string) []string {
	if !fileExists(fileNameA) {
		return nil
	}

	fileT, err := os.Open(fileNameA)
	if err != nil {
		return nil
	}

	defer fileT.Close()

	fileContentT, err := ioutil.ReadAll(fileT)
	if err != nil {
		return nil
	}

	stringList := strings.Split(strings.Replace(string(fileContentT), "\r", "", -1), "\n")

	return stringList
}

func loadMapFromFile(fileNameA string) map[string]string {
	if !fileExists(fileNameA) {
		return nil
	}

	strListT := loadStringList(fileNameA)

	if strListT == nil {
		return nil
	}

	mapT := make(map[string]string)
	for i := range strListT {
		lineT := strListT[i]
		lineListT := strings.SplitN(lineT, "=", 2)
		if (lineListT == nil) || (len(lineListT) < 2) {
			continue
		}
		mapT[lineListT[0]] = lineListT[1]
	}

	return mapT
}

func appendStringToFile(strA string, fileA string) string {
	fileT, err := os.OpenFile(fileA, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return err.Error()
	}

	writerT := bufio.NewWriter(fileT)

	writerT.WriteString(strA)

	writerT.Flush()

	defer fileT.Close()

	return ""
}

func saveString(strA string, fileA string) string {
	file, err := os.Create(fileA)
	if err != nil {
		return err.Error()
	}

	defer file.Close()

	wFile := bufio.NewWriter(file)
	_, err = wFile.WriteString(strA)

	if err != nil {
		return err.Error()
	}

	err = wFile.Flush()

	if err != nil {
		return err.Error()
	}

	return ""
}

func ishex(c byte) bool {
	switch {
	case '0' <= c && c <= '9':
		return true
	case 'a' <= c && c <= 'f':
		return true
	case 'A' <= c && c <= 'F':
		return true
	}
	return false
}

func unhex(c byte) byte {
	switch {
	case '0' <= c && c <= '9':
		return c - '0'
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

func EncodeStringSimple(strA string) string {
	lenT := len(strA)

	hexCount := 0
	for i := 0; i < lenT; i++ {
		v := strA[i]
		if !(((v >= '0') && (v <= '9')) || ((v >= 'a') && (v <= 'z'))) {
			hexCount++
		}
	}

	if hexCount == 0 {
		return strA
	}

	t := make([]byte, lenT+2*hexCount)
	j := 0

	for i := 0; i < lenT; i++ {
		switch v := strA[i]; {
		case !(((v >= '0') && (v <= '9')) || ((v >= 'a') && (v <= 'z'))):
			t[j] = '%'
			t[j+1] = "0123456789ABCDEF"[v>>4]
			t[j+2] = "0123456789ABCDEF"[v&15]
			j += 3
		default:
			t[j] = strA[i]
			j++
		}
	}

	return string(t)
}

func DecodeStringSimple(s string) string {
	n := 0

	for i := 0; i < len(s); {
		switch s[i] {
		case '%':
			n++

			if i+2 >= len(s) || !ishex(s[i+1]) || !ishex(s[i+2]) {
				return s
			}

			i += 3

		default:
			i++
		}
	}

	t := make([]byte, len(s)-2*n)
	j := 0
	for i := 0; i < len(s); {
		switch s[i] {
		case '%':
			t[j] = unhex(s[i+1])<<4 | unhex(s[i+2])
			j++
			i += 3
		default:
			t[j] = s[i]
			j++
			i++
		}
	}
	return string(t)
}

func getFormValueWithDefaultValue(reqA *http.Request, keyA string, defaultA string) string {
	valueT, ok := reqA.Form[keyA]
	if ok {
		return valueT[0]
	} else {
		return defaultA
	}
}

func getSwitchWithDefaultValue(argsA []string, switchStrA string, defaultA string) string {
	tmpStrT := ""
	for _, argT := range argsA {
		if strings.HasPrefix(argT, switchStrA) {
			tmpStrT = argT[len(switchStrA):]
			if strings.HasPrefix(tmpStrT, "\"") && strings.HasSuffix(tmpStrT, "\"") {
				return tmpStrT[1 : len(tmpStrT)-1]
			}

			return tmpStrT
		}

	}

	return defaultA

}

func ifSwitchExists(argsA []string, switchStrA string) bool {
	for _, argT := range argsA {
		if strings.HasPrefix(argT, switchStrA) {
			return true
		}
	}

	return false
}

func downloadUtf8Page(urlA string, postDataA url.Values, timeoutSecsA time.Duration) string {
	client := &http.Client{
		//CheckRedirect: redirectPolicyFunc,
		Timeout: 1000000000 * timeoutSecsA,
	}

	var urlT string
	if !strings.HasPrefix(strings.ToLower(urlA), "http") {
		urlT = "http://" + urlA
	} else {
		urlT = urlA
	}

	var respT *http.Response
	var errT error
	// var req *http.Request

	if postDataA == nil {
		respT, errT = client.Get(urlT)
	} else {
		respT, errT = client.PostForm(urlT, postDataA)
	}

	if errT == nil {
		defer respT.Body.Close()
		if respT.StatusCode != 200 {
			return fmt.Sprintf("failed response status: %v", respT.StatusCode)
		} else {
			body, errT := ioutil.ReadAll(respT.Body)

			if errT == nil {
				return string(body)
			} else {
				return errT.Error()
			}
		}
	} else {
		return errT.Error()
	}
}

type program struct {
	BasePath string
}

func (p *program) Start(s service.Service) error {
	// Start should not block. Do the actual work async.
	// basePathG = p.BasePath
	// logWithTime("basePath: %v", basePathG)
	serviceModeG = true

	go p.run()

	return nil
}

func (p *program) run() {
	go doWork()
}

func (p *program) Stop(s service.Service) error {
	// Stop should not block. Return with a few seconds.
	return nil
}

func doWork() {

	go Svc()

	for {
		select {
		case <-exit:
			os.Exit(0)
			return
		}
	}
}

func stopWork() {

	// logWithTime("Service stop running!")
	exit <- struct{}{}
}

func HttpHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("This is an example server.\n"))
	// fmt.Fprintf(w, "This is an example server.\n")
	// io.WriteString(w, "This is an example server.\n")
}

func shareHandler(w http.ResponseWriter, req *http.Request) {
	codeT := req.RequestURI

	codeT = strings.TrimSpace(strings.TrimPrefix(codeT, "/share/"))

	var rs string

	if codeT == "" {
		rs = "invalid code"

		w.Header().Set("Content-Type", "text/plain;charset=utf-8")
		w.Write([]byte(rs))

		return
	}

	rs, err := loadString(filepath.Join(dataPathG, DecodeStringSimple(codeT)+".txt"))

	if !err {
		rs = fmt.Sprintf("failed: %v", rs)
	}

	w.Header().Set("Content-Type", "text/plain;charset=utf-8")

	w.Write([]byte(rs))
}

func HttpApiHandler(w http.ResponseWriter, req *http.Request) {
	rs := doApi(w, req)
	w.Header().Set("Content-Type", "text/plain;charset=utf-8")

	w.Write([]byte(rs))
}

func doApi(resA http.ResponseWriter, reqA *http.Request) string {
	if reqA == nil {
		return "invalid request"
	}

	reqA.ParseForm()

	reqT := getFormValueWithDefaultValue(reqA, "req", "")

	switch reqT {
	case "":
		return fmt.Sprintf("getx V%v, empty request", versionG)
	case "showstatus", "status":
		return fmt.Sprintf("getx V%v, os: %v, basePathG: %v, dataPathG: %v", versionG, runtime.GOOS, basePathG, dataPathG)
	case "save", "set":
		codeT := getFormValueWithDefaultValue(reqA, "code", "")

		if strings.TrimSpace(codeT) == "" {
			return "invalid code"
		}

		textT := getFormValueWithDefaultValue(reqA, "text", "")

		if strings.TrimSpace(textT) == "" {
			return "content empty"
		}

		if len(textT) > maxClipSizeG {
			return "content exceeds the size limit"
		}

		rs := saveString(textT, filepath.Join(dataPathG, EncodeStringSimple(codeT)+".txt"))

		if rs != "" {
			return fmt.Sprintf("failed: %v", rs)
		}

		return "saved"
	case "load", "get":
		codeT := getFormValueWithDefaultValue(reqA, "code", "")

		if strings.TrimSpace(codeT) == "" {
			return "invalid code"
		}

		rs, err := loadString(filepath.Join(dataPathG, DecodeStringSimple(codeT)+".txt"))

		if !err {
			return fmt.Sprintf("failed: %v", rs)
		}

		return rs
	default:
		return fmt.Sprintf("unknown request: %v", reqA)
	}

	return ""
}

func startHttpServer(portA string) {
	logWithTime("starting http server on port %v...", portA)
	// logWithTime("https port: %v", portA)
	// http.HandleFunc("/", HttpHandler)
	http.HandleFunc("/api", HttpApiHandler)
	http.HandleFunc("/share/", shareHandler)

	// s := &http.Server{
	// 	Addr:           ":"+portA,
	// 	Handler:        HttpApiHandler,
	// 	ReadTimeout:    10 * time.Second,
	// 	WriteTimeout:   10 * time.Second,
	// 	MaxHeaderBytes: 1 << 20,
	// }
	err := http.ListenAndServe(":"+portA, nil)
	if err != nil {
		logWithTime("ListenAndServeHttp: %v\n", err.Error())
		if serviceModeG {
			fmt.Printf("failed to start server: %v", err.Error())
		}
	} else { // won't be reached since code will stop while ListenAndServe succeed
		logWithTime("ListenAndServeHttp: %v", portA)
	}

}

func Svc() {

	if basePathG == "" {
		basePathG = defaultBasePathG

		ensureMakeDirs(basePathG)
	}

	if dataPathG == "" {
		dataPathG = filepath.Join(basePathG, "data")

		ensureMakeDirs(dataPathG)
	}

	logFileG = filepath.Join(basePathG, "getx.log")

	defer func() {
		if v := recover(); v != nil {
			logWithTime("panic in run %v", v)
		}
	}()

	logWithTime("getx V%v", versionG)
	logWithTime("os: %v, basePathG: %v, configFileNameG: %v", runtime.GOOS, basePathG, defaultConfigFileNameG)

	var currentPortG string = defaultPortG
	var ok bool

	cfgFileNameT := filepath.Join(basePathG, defaultConfigFileNameG)
	if fileExists(cfgFileNameT) {
		fileContentT := loadMapFromFile(cfgFileNameT)

		if fileContentT != nil {
			currentPortG, ok = fileContentT["port"]
			if !ok {
				currentPortG = defaultPortG
			}

			dataPathG, ok = fileContentT["dataPath"]
			if !ok {
				dataPathG = filepath.Join(basePathG, "data")
			}
		}
	}

	clipMapLockG.Lock()

	clipMapG = make(map[string]string, maxClipCountG)

	clipMapG["common"] = ""
	clipMapG["0"] = ""
	clipMapG["1"] = ""
	clipMapG["public"] = ""
	clipMapG["broadcast"] = ""
	clipMapG["tmp"] = ""
	clipMapG["test"] = "test123"

	clipMapLockG.Unlock()

	logWithTime("Service started.")
	logWithTime("Using config file: %v", cfgFileNameT)

	go startHttpServer(currentPortG)

}

func initSvc() *service.Service {
	svcConfigT := &service.Config{
		Name:        "txClipSvc",
		DisplayName: "txClipSvc",
		Description: "Clipboard service by TopXeQ V" + versionG,
	}

	prgT := &program{BasePath: basePathG}
	var s, err = service.New(prgT, svcConfigT)

	if err != nil {
		logWithTime("%s unable to start: %s\n", svcConfigT.DisplayName, err)
		return nil
	}

	return &s
}

func runCmd(cmdLineA []string) {
	cmdT := ""

	for _, v := range cmdLineA {
		if !strings.HasPrefix(v, "-") {
			cmdT = v
			break
		}
	}

	var errT error

	basePathG = getSwitchWithDefaultValue(cmdLineA, "-base=", "")

	if strings.TrimSpace(basePathG) == "" {
		basePathG, errT = filepath.Abs(defaultBasePathG)

		if errT != nil {
			fmt.Printf("invalid base path: %v\n", defaultBasePathG)
			return
		}
	}

	verboseT := ifSwitchExists(cmdLineA, "-v")

	ensureMakeDirs(basePathG)

	if !fileExists(basePathG) {
		fmt.Printf("base path not exists: %v, use current directory instead\n", basePathG)
		basePathG = "."
		return
	}

	if !isDirectory(basePathG) {
		fmt.Printf("base path not exists: %v\n", basePathG)
		return
	}

	fmt.Printf("base path: %v\n", basePathG)

	switch cmdT {
	case "version":
		fmt.Printf("txClipSvc V%v", versionG)
	case "", "run":
		s := initSvc()

		if s == nil {
			logWithTime("Failed to init service")
			break
		}

		err := (*s).Run()
		if err != nil {
			logWithTime("Service \"%s\" failed to run.", (*s).String())
		}
	case "get":
		codeT := getSwitchWithDefaultValue(cmdLineA, "-code=", "")

		var ok bool

		var fileMapT map[string]string = nil

		if codeT == "" {
			if fileMapT == nil {
				fileMapT = loadMapFromFile(filepath.Join(basePathG, defaultConfigFileNameG))
			}

			if fileMapT == nil {
				fmt.Printf("invalid code: %v", codeT)
				return
			}

			codeT, ok = fileMapT["code"]

			if !ok {
				fmt.Printf("invalid code: %v", codeT)
				return
			}
		}

		var currentPortG = ""

		portT := getSwitchWithDefaultValue(cmdLineA, "-port=", "")

		if portT == "" {
			if fileMapT == nil {
				fileMapT = loadMapFromFile(filepath.Join(basePathG, defaultConfigFileNameG))
			}

			if fileMapT == nil {
				currentPortG = defaultPortG
			} else {
				currentPortG, ok = fileMapT["port"]

				if !ok {
					currentPortG = defaultPortG
				}
			}

		} else {
			currentPortG = portT
		}

		serverUrlG = getSwitchWithDefaultValue(cmdLineA, "-server=", "")

		if serverUrlG == "" {
			if fileMapT == nil {
				fileMapT = loadMapFromFile(filepath.Join(basePathG, defaultConfigFileNameG))
			}

			if fileMapT == nil {
				fmt.Printf("invalid server url, no confilg file %v", filepath.Join(basePathG, defaultConfigFileNameG))
				return
			}

			serverUrlG, ok = fileMapT["server"]

			if !ok {
				fmt.Printf("invalid server url: %v", serverUrlG)
				return
			}

		}

		if !strings.HasPrefix(strings.ToLower(serverUrlG), "http") {
			serverUrlG = fmt.Sprintf("http://%v:%v/api", serverUrlG, currentPortG)
		}

		if verboseT {
			fmt.Printf("retrieving text from %v, code: %v\n", serverUrlG, codeT)
		}

		postT := url.Values{}

		postT.Set("req", "getClip")
		postT.Set("code", codeT)

		rs := downloadUtf8Page(serverUrlG, postT, 15)

		addLineEndFlagT := ifSwitchExists(cmdLineA, "-withLineEnd") || ifSwitchExists(cmdLineA, "-L") || ifSwitchExists(cmdLineA, "-l")

		if addLineEndFlagT {
			fmt.Println(rs)
		} else {
			fmt.Print(rs)
		}

		clipboard.WriteAll(rs)

		saveString(rs, filepath.Join(basePathG, defaultClipFileNameG))

	case "save", "set":
		codeT := getSwitchWithDefaultValue(cmdLineA, "-code=", "")

		var ok bool

		var fileMapT map[string]string = nil

		if codeT == "" {
			if fileMapT == nil {
				fileMapT = loadMapFromFile(filepath.Join(basePathG, defaultConfigFileNameG))
			}

			if fileMapT == nil {
				fmt.Printf("invalid code: %v", codeT)
				return
			}

			codeT, ok = fileMapT["code"]

			if !ok {
				fmt.Printf("invalid code: %v", codeT)
				return
			}
		}

		var currentPortG = ""

		portT := getSwitchWithDefaultValue(cmdLineA, "-port=", "")

		if portT == "" {
			if fileMapT == nil {
				fileMapT = loadMapFromFile(filepath.Join(basePathG, defaultConfigFileNameG))
			}

			if fileMapT == nil {
				currentPortG = defaultPortG
			} else {
				currentPortG, ok = fileMapT["port"]

				if !ok {
					currentPortG = defaultPortG
				}
			}

		} else {
			currentPortG = portT
		}

		serverUrlG = getSwitchWithDefaultValue(cmdLineA, "-server=", "")

		if serverUrlG == "" {
			if fileMapT == nil {
				fileMapT = loadMapFromFile(filepath.Join(basePathG, defaultConfigFileNameG))
			}

			if fileMapT == nil {
				fmt.Printf("invalid server url, no confilg file %v", filepath.Join(basePathG, defaultConfigFileNameG))
				return
			}

			serverUrlG, ok = fileMapT["server"]

			if !ok {
				fmt.Printf("invalid server url: %v", serverUrlG)
				return
			}

		}

		if !strings.HasPrefix(strings.ToLower(serverUrlG), "http") {
			serverUrlG = fmt.Sprintf("http://%v:%v/api", serverUrlG, currentPortG)
		}

		var textT string
		var err error

		if ifSwitchExists(cmdLineA, "-file") {
			textT, ok = loadString(filepath.Join(basePathG, defaultClipFileNameG))

			if !ok {
				fmt.Printf("failed to load content from clip file")
				return
			}
		} else if textT = getSwitchWithDefaultValue(cmdLineA, "-text=", ""); textT != "" {

		} else if textT, err = clipboard.ReadAll(); err != nil {
			fmt.Printf("could not get text from clipboard: %v", err.Error())
			return
		}

		if verboseT {
			fmt.Printf("saving text to %v, code: %v\n", serverUrlG, codeT)
		}

		postT := url.Values{}

		postT.Set("req", "saveClip")
		postT.Set("code", codeT)
		postT.Set("text", textT)

		rs := downloadUtf8Page(serverUrlG, postT, 15)

		fmt.Print(rs)

	case "installonly":
		s := initSvc()

		if s == nil {
			fmt.Printf("Failed to install")
			break
		}

		err := (*s).Install()
		if err != nil {
			fmt.Printf("Failed to install: %s\n", err)
			return
		}

		fmt.Printf("Service \"%s\" installed.\n", (*s).String())

	case "install":
		s := initSvc()

		if s == nil {
			fmt.Printf("Failed to install")
			break
		}

		fmt.Printf("Installing service \"%v\"...\n", (*s).String())

		err := (*s).Install()
		if err != nil {
			fmt.Printf("Failed to install: %s\n", err)
			return
		}
		fmt.Printf("Service \"%s\" installed.\n", (*s).String())

		fmt.Printf("Starting service \"%v\"...\n", (*s).String())

		err = (*s).Start()
		if err != nil {
			fmt.Printf("Failed to start: %s\n", err)
			return
		}
		fmt.Printf("Service \"%s\" started.\n", (*s).String())
	case "uninstall":
		s := initSvc()

		if s == nil {
			fmt.Printf("Failed to install")
			break
		}

		err := (*s).Stop()
		if err != nil {
			fmt.Printf("Failed to stop: %s\n", err)
		} else {
			fmt.Printf("Service \"%s\" stopped.\n", (*s).String())
		}

		err = (*s).Uninstall()
		if err != nil {
			fmt.Printf("Failed to remove: %s\n", err)
			return
		}
		fmt.Printf("Service \"%s\" removed.\n", (*s).String())
	case "reinstall":
		s := initSvc()

		if s == nil {
			fmt.Printf("Failed to install")
			break
		}

		err := (*s).Stop()
		if err != nil {
			fmt.Printf("Failed to stop: %s\n", err)
		} else {
			fmt.Printf("Service \"%s\" stopped.\n", (*s).String())
		}

		err = (*s).Uninstall()
		if err != nil {
			fmt.Printf("Failed to remove: %s\n", err)
			return
		}
		fmt.Printf("Service \"%s\" removed.\n", (*s).String())

		err = (*s).Install()
		if err != nil {
			fmt.Printf("Failed to install: %s\n", err)
			return
		}
		fmt.Printf("Service \"%s\" installed.\n", (*s).String())

		err = (*s).Start()
		if err != nil {
			fmt.Printf("Failed to start: %s\n", err)
			return
		}
		fmt.Printf("Service \"%s\" started.\n", (*s).String())
	case "start":
		s := initSvc()

		if s == nil {
			fmt.Printf("Failed to install")
			break
		}

		err := (*s).Start()
		if err != nil {
			fmt.Printf("Failed to start: %s\n", err)
			return
		}
		fmt.Printf("Service \"%s\" started.\n", (*s).String())
	case "stop":
		s := initSvc()

		if s == nil {
			fmt.Printf("Failed to install")
			break
		}
		err := (*s).Stop()
		if err != nil {
			fmt.Printf("Failed to stop: %s\n", err)
			return
		}
		fmt.Printf("Service \"%s\" stopped.\n", (*s).String())
	default:
		fmt.Println("unknown command")
		break
	}

}

func main() {

	if strings.HasPrefix(runtime.GOOS, "win") {
		defaultBasePathG = "c:\\getx"
	} else {
		defaultBasePathG = "/getx"
	}

	if len(os.Args) < 2 {
		fmt.Printf("getx V%v is in service(server) mode. Running the application without any arguments will cause it in service mode.\n", versionG)

		serviceModeG = true

		s := initSvc()

		if s == nil {
			logWithTime("Failed to init service")
			return
		}

		err := (*s).Run()
		if err != nil {
			logWithTime("Service \"%s\" failed to run.", (*s).String())
		}

		return
	}

	runCmd(os.Args[1:])

}
