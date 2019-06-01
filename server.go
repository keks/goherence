package goherence

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"

	"github.com/keks/marx"
)

type Server interface {
	RegisterObservable(id string, rf RenderFunc, obv luigi.Observable) Partial
	RegisterLog(id string, rf RenderFunc, obv margaret.Log) (Partial, marx.Worker, error)

	http.Handler
}

func NewServer() Server {
	var (
		mux = http.NewServeMux()
		s   = &server{
			start:     time.Now(),
			partials:   map[string]Partial{},
			httpMux:   mux,
			c: newCacher(),
			l: logLocker{&sync.Mutex{}, "server"},
		}
	)

	mux.Handle("/coherence/cache", s.c)
	mux.HandleFunc("/partial/", s.servePartialHTTP)

	return s
}

type server struct {
	l sync.Locker

	start time.Time

	partials map[string]Partial

	httpMux *http.ServeMux

	c *cacher
}

func (s *server) RegisterObservable(id string, rf RenderFunc, obv luigi.Observable) Partial {
	s.l.Lock()
	defer s.l.Unlock()

	partial := NewObservablePartial(id, rf, obv)
	s.partials[id] = partial

	obv.Register(luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		if err != nil {
			fmt.Printf("obv handler for %q received an error %s\n", id, err)
			return nil
		}

		return s.c.Invalidate(ctx, id)
	}))

	return partial
}

func (s *server) RegisterLog(id string, rf RenderFunc, log margaret.Log) (Partial, marx.Worker, error) {
	s.l.Lock()
	defer s.l.Unlock()

	partial, partialWorker, err := NewLogPartial(id, rf, log)
	if err != nil {
		return nil, nil, err
	}
	s.partials[id] = partial

	seq, err := log.Seq().Value()
	if err != nil {
		return nil, nil, err
	}

	src, err := log.Query(margaret.Live(true), margaret.Gt(seq.(margaret.Seq)), margaret.SeqWrap(true))
	if err != nil {
		return nil, nil, err
	}

	var (
		sink = luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
			if err != nil {
				fmt.Printf("log handler for %q received an error %s\n", id, err)
				return nil
			}

			return s.c.Invalidate(ctx, id + "-latest")
		})

		invalidateWorker = marx.Worker(func(ctx context.Context) error {
			return luigi.Pump(ctx, sink, src)
		})
	)

	return partial, marx.Unite(invalidateWorker, partialWorker), nil
}

func (s *server) servePartialHTTP(w http.ResponseWriter, r *http.Request) {
	s.l.Lock()
	defer s.l.Unlock()

	id := strings.TrimPrefix(r.URL.Path, "/partial/")
	
	p, ok := s.partials[id]
	if !ok {
		http.Error(w, "Not Found", 404)
		return
	}

	p.ServeHTTP(w, r)
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Connection", "keep-alive")

	s.httpMux.ServeHTTP(w, r)
}
