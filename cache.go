package goherence

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"strconv"
	"sync"
	"time"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
)

type Cacher interface {
	RegisterObservable(id string, rf RenderFunc, obv luigi.Observable)
	RegisterLog(id string, rf RenderFunc, obv margaret.Log)

	http.Handler
}

func NewCacher() Cacher {
	var (
		mux = http.NewServeMux()
		unblock, wait = luigi.NewBroadcast()
		c   = &cacher{
			start:     time.Now(),
			entries:   map[string]*Entry{},
			renderers: map[string]RenderFunc{},
			wait: wait,
			unblock: unblock,
			httpMux:   mux,
		}
	)

	mux.HandleFunc("/coherence/cache", c.serveCacheHTTP)
	mux.HandleFunc("/partial/", c.servePartialHTTP)

	return c
}

type cacher struct {
	l sync.Mutex

	start time.Time

	// renderers contains the RenderFuncs for the particular partials
	renderers map[string]RenderFunc

	// entries contains the most recently written Entry of the observable
	// only for the cache page
	// TODO how to generelize this to streams?
	//  	maybe as a linked list?
	//   -> idea: use logs instead of streams,
	//            and query them in the partial
	entries map[string]*Entry

	// used to unblock the cache page when new data arrives
	wait luigi.Broadcast
	unblock luigi.Sink

	// 
	httpMux *http.ServeMux
}

func (c *cacher) RegisterObservable(id string, rf RenderFunc, obv luigi.Observable) {
	c.l.Lock()
	defer c.l.Unlock()

	c.renderers[id] = rf
	c.entries[id] = &Entry{
		Observable: obv,
	}

	obv.Register(luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		if err != nil {
			fmt.Printf("obv handler for %q received an error %s\n", id, err)
			return nil
		}

		c.l.Lock()
		defer c.l.Unlock()

		c.entries[id].ID = id
		c.entries[id].Time = time.Now()

		c.unblock.Pour(ctx, nil)

		return nil
	}))
}

func (c *cacher) RegisterLog(id string, rf RenderFunc, log margaret.Log) {}

func (c *cacher) servePartialHTTP(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/partial/")

	e, ok := c.entries[id]
	if !ok {
		http.Error(w, "Not Found", 404)
		return
	}

	c.renderers[id](e).ServeHTTP(w, r)
}

func getSince(r *http.Request) (int64, *Error) {
	sinceStr := r.URL.Query()["since"][0]
	if sinceStr == "" {
		return 0, newError(`missing query parameter "since"`, nil, 400)
	}

	since, err := strconv.Atoi(sinceStr)
	if err != nil {
		return 0, newError(
			`error parsing query parameter "since"`,
			err, 400)
	}

	return int64(since), nil
}

func (c *cacher) serveCacheHTTP(w http.ResponseWriter, r *http.Request) {
	since, err := getSince(r)
	if err != nil {
		err.ServeHTTP(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)

	type cacheData struct {
		IDs   map[string]int64 `json:"ids"`
		Start int64            `json:"start"`
	}

	data := cacheData{
		make(map[string]int64),
		millis(c.start),
	}

	var wait sync.Mutex

	for {
		for name, entry := range c.entries {
			ms := millis(entry.Time)

			// skip older stuff
			if int64(since) >= ms {
				continue
			}

			data.IDs[name] = ms
		}

		// if we have data, push it!
		if len(data.IDs) > 0 {
			fmt.Println(data)
			break
		}

		// This is a funny construction:
		// The idea is that we take a lock two times (the second time blocks).
		// At the same time, we register a broadcast listener for new data.
		// When new data arrives, we unblock the second call to Lock().
		// The unblocking has to happen in once.Do because the handler
		//     may be called multiple times.
		// the blocking has to happen *outside the global lock* because
		//     otherwise no data can be written.

		// Take lock the first time
		wait.Lock()

		var once sync.Once
		unreg := c.wait.Register(luigi.FuncSink(func(context.Context, interface{}, error) error {
			once.Do(func() {  // only run this code once!
				wait.Unlock() // unlock (and unblock below) once we have data
			})

			return nil
		}))

		c.l.Unlock()  // allow data to be written
		wait.Lock()   // block until the closure above is run
		unreg()       // unregister the handler
		wait.Unlock() // clean up and prepare for another iteration
		c.l.Lock()    // take the global lock again
	}

	// this only prints an error if there was one
	wrapError("error encoding json", enc.Encode(data), 500).ServeHTTP(w, r)
}

func (c *cacher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Connection", "keep-alive")
	c.l.Lock()
	defer c.l.Unlock()

	c.httpMux.ServeHTTP(w, r)
}
