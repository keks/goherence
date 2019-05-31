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
	Wat, Line, Send goherence.Partial
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

	obvRenderFunc := goherence.TemplateRenderFunc2(tpl, "simple-obv")


	lineObv, lineWork := LineObservable(os.Stdin)
	linePartial := goherence.NewObservablePartial("line", obvRenderFunc, lineObv)

	sendObv := luigi.NewObservable("send something!")
	sendPartial := goherence.NewObservablePartial("send", obvRenderFunc, sendObv)

	watObv := luigi.NewObservable(0)
	watPartial := goherence.NewObservablePartial("wat", obvRenderFunc, watObv)

	srv := goherence.NewServer()
	srv.RegisterObservable("wat", obvRenderFunc, watObv)
	srv.RegisterObservable("line", obvRenderFunc, lineObv)
	srv.RegisterObservable("send", obvRenderFunc, sendObv)

	mux := http.NewServeMux()
	mux.Handle("/coherence.js", fileServer("assets/coherence.js"))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			srv.ServeHTTP(w, r)
			return 
		}

		if r.Method == "POST" {
			v := r.FormValue("msg")
			fmt.Println("received value for send:", v)
			err := sendObv.Set(v)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
		}

		should := func(v interface{}, err error) interface{} {
			if err != nil {
				return err.Error()
			}

			return v
		}

		err := tpl.ExecuteTemplate(w, "root", &AppState{
			goherence.TemplateData{
				Partial: watPartial,
				Value: should(watObv.Value()),
			},
			goherence.TemplateData{
				Partial: linePartial,
				Value: should(lineObv.Value()),
			},
			goherence.TemplateData{
				Partial: sendPartial,
				Value: should(sendObv.Value()),
			},
		})
		if err != nil {
			http.Error(w, err.Error(), 500)
		}
	})

	go lineWork()
	go func() {
		i := 0
		for range time.Tick(2*time.Second) {
			err := watObv.Set(i)
			if err != nil {
				fmt.Println("error setting wat:", err)
			}
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
