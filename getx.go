package main

import (
	"fmt"
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
	"github.com/topxeq/tk"
)

var versionG string = "0.96a"

var defaultPortG string = "7468"
var defaultSslPortG string = "7469"
var defaultBasePathG string
var defaultConfigFileNameG string = "getx.cfg"
var defaultClipFileNameG string = "clip.txt"

var clipMapG map[string]string = nil
var clipMapLockG sync.Mutex

var maxClipCountG int = 100 + 1
var maxClipSizeG int = 832768
var maxImageSizeG int = 8000000

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

	tk.AppendStringToFile(fmt.Sprintf(fmt.Sprintf("[%v] ", time.Now())+formatA+"\n", argsA...), logFileG)
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

var htmlTemplateG = ``

func HttpHandler(w http.ResponseWriter, reqA *http.Request) {
	reqA.ParseForm()

	// fmt.Printf("%#v\n", reqA.Form)

	reqT := strings.ToLower(tk.GetFormValueWithDefaultValue(reqA, "req", ""))

	codeT := ""
	textT := ""
	imageTextT := ""
	resultT := ""

	// fmt.Printf("req: %#v, code: %v\n", reqT, codeT)

	switch reqT {
	case "load", "get", "share":
		codeT = strings.TrimSpace(tk.GetFormValueWithDefaultValue(reqA, "code", ""))

		if codeT == "" {
			textT = ""
			resultT = fmt.Sprintf(`<span style="color: red;">failed: %v</span>`, `empty code`)
			break
		}

		matchT := tk.RegFindFirst(codeT, `^TXENC(.*?)TXEND`, 1)

		if !tk.IsErrorString(matchT) {
			codeT = tk.SplitN(codeT, "TXEND", 2)[1]
		}

		rs, ok := tk.LoadStringFromFileB(filepath.Join(dataPathG, tk.EncodeStringSimple(codeT)+".txt"))

		if !ok {
			textT = ""
			resultT = fmt.Sprintf(`<span style="color: red;">failed: %v</span>`, rs)
			break
		}

		if !tk.IsErrorString(matchT) {
			rs = tk.DecryptStringByTXDEE(rs, matchT)
		}

		textT = rs

		rs, ok = tk.LoadStringFromFileB(filepath.Join(dataPathG, tk.EncodeStringSimple(codeT)+".img"))

		if ok {
			imageTextT = rs
		}

		resultT = ""

	case "save", "set":
		codeT = strings.TrimSpace(tk.GetFormValueWithDefaultValue(reqA, "code", ""))

		if codeT == "" {
			textT = ""
			resultT = fmt.Sprintf(`<span style="color: red;">failed: %v</span>`, `empty code`)
			break
		}

		if strings.ContainsAny(codeT, ".:&=/\\ \n\r") {
			textT = ""
			resultT = fmt.Sprintf(`<span style="color: red;">failed: %v</span>`, `invalid character(s) in code`)
			break
		}

		textT = tk.GetFormValueWithDefaultValue(reqA, "text", "")

		imageTextT = strings.TrimSpace(tk.GetFormValueWithDefaultValue(reqA, "mainImg", ""))

		if textT == "" && imageTextT == "" {
			textT = ""
			resultT = fmt.Sprintf(`<span style="color: red;">failed: %v</span>`, `empty content`)
			break
		}

		if len(textT) > maxClipSizeG {
			lenT := len(textT)
			textT = ""
			resultT = fmt.Sprintf(`<span style="color: red;">failed: %v(%v/%v)</span>`, `content exceeds the size limit`, lenT, maxClipSizeG)
			break
		}

		matchT := tk.RegFindFirst(codeT, `^TXENC(.*?)TXEND`, 1)

		if !tk.IsErrorString(matchT) {
			codeT = tk.SplitN(codeT, "TXEND", 2)[1]

			textT = tk.EncryptStringByTXDEE(textT, matchT)
		}

		if len(imageTextT) > maxImageSizeG {
			lenT := len(imageTextT)
			textT = ""
			resultT = fmt.Sprintf(`<span style="color: red;">failed: %v(%v/%v)</span>`, `content exceeds the size limit`, lenT, maxImageSizeG)
			break
		}

		if imageTextT != "" {
			tk.SaveStringToFile(imageTextT, filepath.Join(dataPathG, tk.EncodeStringSimple(codeT)+".img"))
		}

		rs := tk.SaveStringToFile(textT, filepath.Join(dataPathG, tk.EncodeStringSimple(codeT)+".txt"))

		if rs != "" {
			textT = ""
			resultT = fmt.Sprintf(`<span style="color: red;">failed: %v</span>`, rs)
			break
		}

		linkT := "http://" + reqA.Host + "/share/" + codeT
		linkWebT := "http://" + reqA.Host + "/?req=get&code=" + codeT

		resultT = `share link: <a target="_blank" href="` + linkT + `">` + `share by plain text` + `</a><br /><a target="_blank" href="` + linkWebT + `">` + `share by WEB page` + `</a>`

	}

	formatA := tk.GetFormValueWithDefaultValue(reqA, "format", "")

	if formatA == "html" {
		w.Header().Set("Content-Type", "text/html;charset=utf-8")
		w.Write([]byte(textT))
		return
	} else if formatA == "md" {
		htmlTemplateG, ok := tk.LoadStringFromFileB(filepath.Join(basePathG, "mdtmpl.html"))

		if ok {
			text1T := strings.Replace(textT, "\r", "", -1)
			text1T = strings.Replace(text1T, "\n", "#CR#", -1)
			text1T = strings.Replace(text1T, `"`, "#DQ#", -1)

			strT := strings.Replace(htmlTemplateG, "<TXMDDATA></TXMDDATA>", `var mdT = "`+text1T+`";`, -1)

			w.Header().Set("Content-Type", "text/html;charset=utf-8")
			w.Write([]byte(strT))
			return
		}
	}

	w.Header().Set("Content-Type", "text/html;charset=utf-8")

	if true { //htmlTemplateG == "" {
		htmlTemplateG, _ = tk.LoadStringFromFileB(filepath.Join(basePathG, "htmltmpl.html"))
	}

	strT := strings.Replace(htmlTemplateG, "{{.CODE}}", codeT, -1)
	strT = strings.Replace(strT, "{{.TEXT}}", textT, -1)
	strT = strings.Replace(strT, "{{.RESULTMSG}}", resultT, -1)
	strT = strings.Replace(strT, "{{.MAINIMG}}", imageTextT, -1)

	w.Write([]byte(strT))
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

	if tk.RegContains(codeT, `%[A-F0-9][A-F0-9]`) {
		codeT = tk.UrlDecode(codeT)
	}

	rs, err := tk.LoadStringFromFileB(filepath.Join(dataPathG, tk.EncodeStringSimple(codeT)+".txt"))

	if !err {
		rs = fmt.Sprintf("failed: %v", rs)
	}

	w.Header().Set("Content-Type", "text/plain;charset=utf-8")

	w.Write([]byte(rs))
}

func codeHandler(w http.ResponseWriter, req *http.Request) {
	codeT := req.RequestURI

	codeT = strings.TrimSpace(strings.TrimPrefix(codeT, "/code/"))

	var rs string

	if codeT == "" {
		rs = "invalid code"

		w.Header().Set("Content-Type", "text/plain;charset=utf-8")
		w.Write([]byte(rs))

		return
	}

	if tk.RegContains(codeT, `%[A-F0-9][A-F0-9]`) {
		codeT = tk.UrlDecode(codeT)
	}

	rs, ok := tk.LoadStringFromFileB(filepath.Join(dataPathG, tk.EncodeStringSimple(codeT)+".txt"))

	var textT, resultT, imageTextT string

	if !ok {
		textT = ""
		resultT = fmt.Sprintf(`<span style="color: red;">failed: %v</span>`, rs)
	} else {
		textT = rs

		rs, ok = tk.LoadStringFromFileB(filepath.Join(dataPathG, tk.EncodeStringSimple(codeT)+".img"))

		if ok {
			imageTextT = rs
		}

		resultT = ""

	}

	w.Header().Set("Content-Type", "text/html;charset=utf-8")

	if true { //htmlTemplateG == "" {
		htmlTemplateG, _ = tk.LoadStringFromFileB(filepath.Join(basePathG, "htmltmpl.html"))
	}

	strT := strings.Replace(htmlTemplateG, "{{.CODE}}", codeT, -1)
	strT = strings.Replace(strT, "{{.TEXT}}", textT, -1)
	strT = strings.Replace(strT, "{{.RESULTMSG}}", resultT, -1)
	strT = strings.Replace(strT, "{{.MAINIMG}}", imageTextT, -1)

	w.Write([]byte(strT))

}

func mdHandler(w http.ResponseWriter, req *http.Request) {
	codeT := req.RequestURI

	codeT = strings.TrimSpace(strings.TrimPrefix(codeT, "/md/"))

	var rs string

	if codeT == "" {
		rs = "invalid code"

		w.Header().Set("Content-Type", "text/plain;charset=utf-8")
		w.Write([]byte(rs))

		return
	}

	if tk.RegContains(codeT, `%[A-F0-9][A-F0-9]`) {
		codeT = tk.UrlDecode(codeT)
	}

	textT, ok := tk.LoadStringFromFileB(filepath.Join(dataPathG, tk.EncodeStringSimple(codeT)+".txt"))

	htmlTemplateG, ok := tk.LoadStringFromFileB(filepath.Join(basePathG, "mdtmpl.html"))

	if ok {
		text1T := strings.Replace(textT, "\r", "", -1)
		text1T = strings.Replace(text1T, "\n", "#CR#", -1)
		text1T = strings.Replace(text1T, `"`, "#DQ#", -1)

		strT := strings.Replace(htmlTemplateG, "<TXMDDATA></TXMDDATA>", `var mdT = "`+text1T+`";`, -1)

		w.Header().Set("Content-Type", "text/html;charset=utf-8")
		w.Write([]byte(strT))
		return
	}

	w.Header().Set("Content-Type", "text/html;charset=utf-8")
	w.Write([]byte("failed"))
	return

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

	reqT := tk.GetFormValueWithDefaultValue(reqA, "req", "")

	if resA != nil {
		resA.Header().Set("Access-Control-Allow-Origin", "*")
		resA.Header().Set("Content-Type", "text/plain;charset=utf-8")
	}

	switch reqT {
	case "":
		return fmt.Sprintf("getx V%v, empty request", versionG)
	case "showstatus", "status":
		return fmt.Sprintf("getx V%v, os: %v, basePathG: %v, dataPathG: %v", versionG, runtime.GOOS, basePathG, dataPathG)
	case "save", "set":
		codeT := strings.TrimSpace(tk.GetFormValueWithDefaultValue(reqA, "code", ""))

		if codeT == "" {
			return "invalid code"
		}

		if strings.ContainsAny(codeT, ".:&=/\\ \n\r") {
			return `invalid character(s) in code`
		}

		textT := tk.GetFormValueWithDefaultValue(reqA, "text", "")

		if strings.TrimSpace(textT) == "" {
			return "content empty"
		}

		if len(textT) > maxClipSizeG {
			return "content exceeds the size limit"
		}

		rs := tk.SaveStringToFile(textT, filepath.Join(dataPathG, tk.EncodeStringSimple(codeT)+".txt"))

		if rs != "" {
			return fmt.Sprintf("failed: %v", rs)
		}

		withLinkT := tk.GetFormValueWithDefaultValue(reqA, "link", "")

		if withLinkT != "" {
			return "saved, share link: http://" + reqA.Host + "/share/" + codeT
		}

		return "saved"
	case "load", "get":
		codeT := tk.GetFormValueWithDefaultValue(reqA, "code", "")

		if strings.TrimSpace(codeT) == "" {
			return "invalid code"
		}

		rs, errT := tk.LoadStringFromFileE(filepath.Join(dataPathG, tk.EncodeStringSimple(codeT)+".txt"))

		if errT != nil {
			return fmt.Sprintf("failed: %v", errT.Error())
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
	// http.HandleFunc("/api", HttpApiHandler)
	// http.HandleFunc("/share/", shareHandler)
	// http.HandleFunc("/code/", codeHandler)
	// http.HandleFunc("/md/", mdHandler)

	// http.HandleFunc("/", HttpHandler)
	// // s := &http.Server{
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

func startHttpsServer(portA string) {
	logWithTime("starting https server on port %v...", portA)
	// logWithTime("https port: %v", portA)
	// http.HandleFunc("/api", HttpApiHandler)
	// http.HandleFunc("/share/", shareHandler)
	// http.HandleFunc("/code/", codeHandler)
	// http.HandleFunc("/md/", mdHandler)

	// http.HandleFunc("/", HttpHandler)
	// s := &http.Server{
	// 	Addr:           ":"+portA,
	// 	Handler:        HttpApiHandler,
	// 	ReadTimeout:    10 * time.Second,
	// 	WriteTimeout:   10 * time.Second,
	// 	MaxHeaderBytes: 1 << 20,
	// }
	err := http.ListenAndServeTLS(":"+portA, filepath.Join(basePathG, "server.crt"), filepath.Join(basePathG, "server.key"), nil)
	if err != nil {
		logWithTime("ListenAndServeHttps: %v\n", err.Error())
		if serviceModeG {
			fmt.Printf("failed to start https server: %v", err.Error())
		}
	} else { // won't be reached since code will stop while ListenAndServe succeed
		logWithTime("ListenAndServeHttps: %v", portA)
	}

}

func Svc() {

	if basePathG == "" {
		basePathG = defaultBasePathG

		tk.EnsureMakeDirs(basePathG)
	}

	if dataPathG == "" {
		dataPathG = filepath.Join(basePathG, "data")

		tk.EnsureMakeDirs(dataPathG)
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
	var currentSslPortG string = defaultSslPortG
	var ok bool

	cfgFileNameT := filepath.Join(basePathG, defaultConfigFileNameG)
	if tk.IfFileExists(cfgFileNameT) {
		fileContentT := tk.LoadSimpleMapFromFile(cfgFileNameT)

		if fileContentT != nil {
			currentPortG, ok = fileContentT["port"]
			if !ok {
				currentPortG = defaultPortG
			}

			currentSslPortG, ok = fileContentT["sslPort"]
			if !ok {
				currentSslPortG = defaultSslPortG
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

	http.HandleFunc("/api", HttpApiHandler)
	http.HandleFunc("/share/", shareHandler)
	http.HandleFunc("/code/", codeHandler)
	http.HandleFunc("/md/", mdHandler)

	http.HandleFunc("/", HttpHandler)
	// s := &http.Server{

	go startHttpServer(currentPortG)
	go startHttpsServer(currentSslPortG)

}

func initSvc() *service.Service {
	svcConfigT := &service.Config{
		Name:        "getxSvc",
		DisplayName: "getxSvc",
		Description: "getx service by TopXeQ V" + versionG,
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

	basePathG = tk.GetSwitchWithDefaultValue(cmdLineA, "-base=", "")

	if strings.TrimSpace(basePathG) == "" {
		basePathG, errT = filepath.Abs(defaultBasePathG)

		if errT != nil {
			fmt.Printf("invalid base path: %v\n", defaultBasePathG)
			return
		}
	}

	verboseT := tk.IfSwitchExists(cmdLineA, "-v")

	tk.EnsureMakeDirs(basePathG)

	if !tk.IfFileExists(basePathG) {
		fmt.Printf("base path not exists: %v, use current directory instead\n", basePathG)
		basePathG = "."
		return
	}

	if !tk.IsDirectory(basePathG) {
		fmt.Printf("base path not exists: %v\n", basePathG)
		return
	}

	// fmt.Printf("base path: %v\n", basePathG)

	switch cmdT {
	case "version":
		fmt.Printf("getx V%v", versionG)
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
	case "get", "load":
		codeT := tk.GetSwitchWithDefaultValue(cmdLineA, "-code=", "public")

		currentPortG := tk.GetSwitchWithDefaultValue(cmdLineA, "-port=", "7468")

		serverUrlG = tk.GetSwitchWithDefaultValue(cmdLineA, "-server=", "getx.topget.org")

		if !strings.HasPrefix(strings.ToLower(serverUrlG), "http") {
			serverUrlG = fmt.Sprintf("http://%v:%v/api", serverUrlG, currentPortG)
		}

		if verboseT {
			fmt.Printf("retrieving text from %v, code: %v\n", serverUrlG, codeT)
		}

		postT := url.Values{}

		postT.Set("req", "get")
		postT.Set("code", codeT)

		rs := tk.DownloadPageUTF8(serverUrlG, postT, "", 15)

		ifClipT := tk.IfSwitchExists(cmdLineA, "-clip")

		if ifClipT {
			clipboard.WriteAll(rs)
		}

		saveFileT := tk.GetSwitchWithDefaultValue(cmdLineA, "-file=", "")

		if saveFileT != "" {
			errStrT := tk.SaveStringToFile(rs, saveFileT)

			if errStrT != "" {
				fmt.Printf("failed to save file (%v): %v", saveFileT, errStrT)
			}

			break
		}

		noLineEndFlagT := tk.IfSwitchExists(cmdLineA, "-noLineEnd") || tk.IfSwitchExists(cmdLineA, "-nl") || tk.IfSwitchExists(cmdLineA, "-NL")

		if noLineEndFlagT {
			fmt.Print(rs)
		} else {
			fmt.Println(rs)
		}

	case "save", "set":
		codeT := strings.TrimSpace(tk.GetSwitchWithDefaultValue(cmdLineA, "-code=", "public"))

		if codeT == "" {
			fmt.Printf(`empty code`)
			return
		}

		if strings.ContainsAny(codeT, ".:&=/\\ \n\r") {
			fmt.Printf(`invalid character(s) in code`)
			return
		}

		currentPortG := tk.GetSwitchWithDefaultValue(cmdLineA, "-port=", "7468")

		serverUrlG = tk.GetSwitchWithDefaultValue(cmdLineA, "-server=", "getx.topget.org")

		if !strings.HasPrefix(strings.ToLower(serverUrlG), "http") {
			serverUrlG = fmt.Sprintf("http://%v:%v/api", serverUrlG, currentPortG)
		}

		var textT string
		// var ok bool
		var errT error

		if fileNameT := tk.GetSwitchWithDefaultValue(cmdLineA, "-file=", ""); fileNameT != "" {

			textT, errT = tk.LoadStringFromFileE(fileNameT)

			if errT != nil {
				fmt.Printf("failed to load content from file: %v", errT.Error())
				return
			}
		} else if textT = tk.GetSwitchWithDefaultValue(cmdLineA, "-text=", ""); textT != "" {

		} else if textT, errT = clipboard.ReadAll(); errT != nil {
			fmt.Printf("could not get text from clipboard: %v", errT.Error())
			return
		}

		if verboseT {
			fmt.Printf("saving text to %v, code: %v\n", serverUrlG, codeT)
		}

		postT := url.Values{}

		postT.Set("req", "save")
		postT.Set("code", codeT)
		postT.Set("text", textT)

		rs := tk.DownloadPageUTF8(serverUrlG, postT, "", 15)

		noLineEndFlagT := tk.IfSwitchExists(cmdLineA, "-noLineEnd") || tk.IfSwitchExists(cmdLineA, "-nl") || tk.IfSwitchExists(cmdLineA, "-NL")

		if noLineEndFlagT {
			fmt.Print(rs)
		} else {
			fmt.Println(rs)
		}

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
