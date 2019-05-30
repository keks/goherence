package main

import (
	"bufio"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"time"

	"go.cryptoscope.co/goherence"
	"go.cryptoscope.co/luigi"
)

const (
	BindAddr = ":3001"

	TemplateRoot = `

<html>
<script type="text/javascript" src="coherence.js"></script>
<body>
<div>regular div</div>

{{- block "simple-obv" .Line -}}
<div {{ attrs . }}> {{ .ID }}: {{ .Value }} </div>
{{end -}}

{{ template "simple-obv" .Wat }}
{{ template "simple-obv" .Send }}
<form method="POST">
	<input type="text" name="msg">
	<input type="submit" value="send it">
</form>
`
)

type AppState struct {
	Wat, Line, Send *goherence.Entry
}

func fileServer(name string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, name)
	})
}

func main() {
	tpl := template.Must(
		template.New("root").
		Funcs(goherence.TemplateFuncs).
		Parse(TemplateRoot))

	cchr := goherence.NewCacher()

	lineObv, lineWork := LineObservable(os.Stdin)
	lineEntry := &goherence.Entry{
		ID: "line",
		Time: time.Now(),
		Observable: lineObv,
	}

	sendObv := luigi.NewObservable("send something!")
	sendEntry := &goherence.Entry{
		ID: "send",
		Time: time.Now(),
		Observable: sendObv,
	}

	watObv := luigi.NewObservable(0)
	watEntry := &goherence.Entry{
		ID: "wat",
		Time: time.Now(),
		Observable: watObv,
	}

	cchr.RegisterObservable("wat", goherence.TemplateRenderFunc(tpl, "simple-obv"), watObv)
	cchr.RegisterObservable("line", goherence.TemplateRenderFunc(tpl, "simple-obv"), lineObv)
	cchr.RegisterObservable("send", goherence.TemplateRenderFunc(tpl, "simple-obv"), sendObv)

	mux := http.NewServeMux()
	mux.Handle("/coherence.js", fileServer("assets/coherence.js"))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			cchr.ServeHTTP(w, r)
			return 
		}

		if r.Method == "POST" {
			v := r.FormValue("msg")
			fmt.Println("received value for send:", v)
			sendObv.Set(v)
		}

		tpl.ExecuteTemplate(w, "root", &AppState{watEntry, lineEntry, sendEntry})
	})

	go lineWork()
	go func() {
		i := 0
		for range time.Tick(20*time.Second) {
			watObv.Set(i)
			i++
		}
	}()

	err := http.ListenAndServe(BindAddr, mux)
	if err != nil {
		panic(err)
	}
}

func LineObservable(r io.Reader) (luigi.Observable, func()) {
	o := luigi.NewObservable("...line...")

	return o, func() {
		br := bufio.NewReader(r)
		for {
			str, err := br.ReadString('\n')
			if err != nil {
				o.Set("error reading: " + err.Error())
				return
			}
			o.Set(str)
		}
	}
}
