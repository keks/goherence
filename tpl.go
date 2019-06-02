package goherence

import (
	"fmt"
	"html/template"
	"io"
	"net/http"
)

var TemplateFuncs template.FuncMap

func init() {
	TemplateFuncs = template.FuncMap{
		"attrs": func(e Partial) template.HTMLAttr {
			return template.HTMLAttr(
				fmt.Sprintf(
					`data-id="%s" data-href="/%s" data-ts="%d"`, e.ID(), e.ID(), millis(e.Timestamp())))
		},
		"millis": millis,
	}
}

type TemplateData struct {
	Partial

	Value interface{}
}

func writerToHandler(wt io.WriterTo) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := wt.WriteTo(w)
		if err != nil {
			http.Error(w, err.Error(), 500)
		}
	})
}

type RenderFunc func(interface{}, Partial) io.WriterTo

func TemplateRenderFunc(tpl *template.Template, name string) RenderFunc {
	return func(v interface{}, p Partial) io.WriterTo {
		return writerToFunc(func(w io.Writer) (int64, error) {
			fmt.Printf("rendering %s with template %s with value %v\n", p.ID(), name, v)

			cw := &countWriter{w: w}

			err := tpl.ExecuteTemplate(cw, name, TemplateData{
				Partial: p,
				Value:   v,
			})

			return cw.n, err
		})
	}
}
