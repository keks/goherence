package main

import (
	"bufio"
	"context"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"time"

	"go.cryptoscope.co/goherence"
	"go.cryptoscope.co/luigi"
	memmarge "go.cryptoscope.co/margaret/mem"

	"github.com/keks/marx"
)

const (
	BindAddr = ":3001"

	TemplateRoot = `
{{- define "simple-obv" -}}
<div {{ attrs . }}> {{ .ID }}: {{ .Value }} </div>
{{ end -}}

{{ define "simple-log" -}}
<div class="{{.ID}}">
	{{ .Timestamp }} {{ .Value }} 
</div>
{{ end -}}

{{ define "simple-form" -}}
<form method="POST" action="endpoint/{{ . }}" data-reset="true">
	<input type="text" name="msg">
	<input type="submit" value="{{ . }} it">
</form>
{{ end -}}

<html>
<head>
	<script type="text/javascript" src="coherence.js"></script>
</head>
<body>
<div>regular div</div>
{{ .Line.HTML -}}
{{ .Wat.HTML -}}
{{ .Send.HTML -}}
{{ template "simple-form" "send" -}}
{{.Chat.HTML -}}
{{ template "simple-form" "chat" -}}
`
)

type AppState struct {
	Wat, Line, Send, Chat goherence.Partial
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

	obvRenderFunc := goherence.TemplateRenderFunc(tpl, "simple-obv")
	chatRenderFunc := goherence.TemplateRenderFunc(tpl, "simple-log")

	lineObv, lineWork := LineObservable(os.Stdin)
	sendObv := luigi.NewObservable("send something!")
	watObv := luigi.NewObservable(0)


	srv := goherence.NewServer()
	watPartial := srv.RegisterObservable("wat", obvRenderFunc, watObv)
	linePartial := srv.RegisterObservable("line", obvRenderFunc, lineObv)
	sendPartial := srv.RegisterObservable("send", obvRenderFunc, sendObv)

	chatLog := memmarge.New()
	chatPartial, chatWork, err := 	srv.RegisterLog("chat", chatRenderFunc, chatLog)
	if err != nil {
		panic(err)
	}

	var httpSrv *http.Server

	epMux := http.NewServeMux()
	epMux.HandleFunc("/send", func(w http.ResponseWriter, r *http.Request) {
		v := r.FormValue("msg")
		fmt.Println("received value for send:", v)
		err := sendObv.Set(v)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	})
	epMux.HandleFunc("/chat", func(w http.ResponseWriter, r *http.Request) {
		v := r.FormValue("msg")
		fmt.Println("received value for chat:", v)
		_, err := chatLog.Append(v)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	})
	epMux.HandleFunc("/halt", func(w http.ResponseWriter, r *http.Request) {
		go httpSrv.Shutdown(context.Background())
		fmt.Fprintf(w, "shutting down")
	})

	mux := http.NewServeMux()
	mux.Handle("/coherence.js", fileServer("assets/coherence.js"))
	mux.HandleFunc("/endpoint/",  func(w http.ResponseWriter, r *http.Request) {
		http.StripPrefix("/endpoint", epMux).ServeHTTP(w, r)
		http.Redirect(w, r, "/", http.StatusFound)
	})
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
			goherence.TemplateData{
				Partial: chatPartial,
				Value: "????",
			},
		})
		if err != nil {
			http.Error(w, err.Error(), 500)
		}
	})

	httpSrv = &http.Server{
		Addr: BindAddr,
		Handler: mux,
	}

	watWork := marx.Worker(func(ctx context.Context) error {
		defer fmt.Println("wat on strike")

		ticker := time.Tick(5*time.Second)
		i := 0
		for  {
			select{
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker:
			}

			fmt.Println("wat: setting...")
			err := watObv.Set(i)
			if err != nil {
				return err
			}
			fmt.Println("wat: new value set.")
			i++
		}

		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	httpWork := marx.Worker(func(ctx context.Context) error {
		defer fmt.Println("http done, cancel called")
		defer cancel()
		return httpSrv.ListenAndServe()
	})

	workers := marx.Unite(
		httpWork,
		chatWork,
		lineWork,
		watWork,
	)

	if err := workers(ctx); err != nil {
		panic(err)
	}
}

func LineObservable(r io.Reader) (luigi.Observable, func(ctx context.Context) error) {
	o := luigi.NewObservable("...line...")

	return o, func(ctx context.Context) error {
		defer fmt.Println("line worker on strike")
		br := bufio.NewReader(r)
		for {
			select{
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			str, err := br.ReadString('\n')
			if err != nil {
				return err
			}

			err = o.Set(str)
			if err != nil {
				return err
			}
		}
	}
}
