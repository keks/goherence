package goherence

import (
	"fmt"
	"html/template"
	"net/http"
	"time"
)

var TemplateFuncs template.FuncMap

func millis(t time.Time) int64 {
	return t.UnixNano() / 1000
}

func init() {
	TemplateFuncs = template.FuncMap{
		"attrs": func(e Entry) template.HTMLAttr {
			return template.HTMLAttr(
				fmt.Sprintf(
					`data-id="%s" data-href="/%s" data-ts="%d"`, e.ID, e.ID, millis(e.Time)))
		},
		"millis": millis,
	}
}

type RenderFunc func(*Entry) http.Handler
type RenderFunc2 func(interface{}) http.Handler

func TemplateRenderFunc(tpl *template.Template, name string) RenderFunc {
	return func(e *Entry) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Printf("rendering %s with template %s with entry %v\n", e.ID, name, e.Value())
			err := tpl.ExecuteTemplate(w, name, e)
			if err != nil {
				http.Error(w, err.Error(), 500)
			}
		})
	}
}
