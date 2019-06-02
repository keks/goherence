package goherence

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.cryptoscope.co/luigi"
)

type Server interface {
	RegisterPartial(Partial) func()

	http.Handler
}

func NewServer() Server {
	var (
		mux = http.NewServeMux()
		s   = &server{
			start:    time.Now(),
			partials: map[string]Partial{},
			httpMux:  mux,
			c:        newCacher(),
			l:        &sync.Mutex{},
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

func (s *server) RegisterPartial(p Partial) func() {
	s.l.Lock()
	defer s.l.Unlock()

	s.partials[p.ID()] = p

	unreg := p.Register(luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		fmt.Printf("invalidate sink for %s called with value %v and error %v\n",
			p.ID(), v, err)

		if err != nil {
			return nil
		}

		return s.c.Invalidate(ctx, p.InvalidateID())
	}))

	return unreg
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
