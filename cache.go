package goherence

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"go.cryptoscope.co/luigi"
)

type logLocker struct {
	l sync.Locker
	name string
}

func (ll logLocker) Lock() {
	fmt.Printf("wait for lock %q\n", ll.name)
	ll.l.Lock()
	fmt.Printf("acquired lock %q\n", ll.name)
}

func (ll logLocker) Unlock() {
	ll.l.Unlock()
	fmt.Printf("released lock %q\n", ll.name)
}

type cacher struct {
	l sync.Locker

	// used to unblock the cache page when new data arrives
	wait luigi.Broadcast
	unblock luigi.Sink
	
	start int64
	times map[string]int64

	cacheData cacheData
}

type cacheLine struct {
	id string
	// ms is the number if milliseconds passed since 1970-01-01.
	// sounds crazy if you put it that way...
	ms int64
}

type cacheData struct {
	IDs   map[string]int64 `json:"ids"`
	Start int64            `json:"start"`
}

func newCacher() *cacher {
	var (
		unblock, wait = luigi.NewBroadcast()
		c = &cacher {
			wait: wait,
			unblock: unblock,
			cacheData: cacheData{
				IDs: make(map[string]int64),
			},
			//l: logLocker{&sync.Mutex{}, "cache"},
			l: &sync.Mutex{},
		}
	)
	
	return c
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

func (c *cacher) Invalidate(ctx context.Context, id string) error {
	ms := millis(time.Now())

	func () {
		c.l.Lock()
		defer c.l.Unlock()

		c.cacheData.IDs[id] = millis(time.Now())
	}()

	return c.unblock.Pour(ctx, cacheLine{
		id: id,
		ms: ms,
	})
}

func (c *cacher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	since, sinceErr := getSince(r)
	if sinceErr != nil {
		sinceErr.ServeHTTP(w, r)
		return
	}

	data := cacheData{
		IDs: make(map[string]int64),
		Start: c.start,
	}

	c.l.Lock()
	defer c.l.Unlock()

	for id, ms := range c.cacheData.IDs {
		fmt.Println(id, ms, since)
		if ms > since {
			data.IDs[id] = ms
		}
	}

	// no new data available! wait for more
	if len(data.IDs) == 0 {
		fmt.Println("no new data available! wait for more")
		// This is a funny construction:
		// The idea is that we take a lock two times (the second time blocks).
		// At the same time, we register a broadcast listener for new data.
		// When new data arrives, we unblock the second call to Lock().
		// The unblocking has to happen in once.Do because the handler
		//     may be called multiple times.
		// the blocking has to happen *outside the global lock* because
		//     otherwise no data can be written.

		var wait sync.Mutex

		// Take lock the first time
		wait.Lock()

		var (
			line cacheLine
		)

		var once sync.Once
		unreg := c.wait.Register(luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
			fmt.Println("waiting for once")
			once.Do(func() {  // only run this code once!
				wait.Unlock() // unlock (and unblock below) once we have data
			})

			// usually you don't do that, but we
			// don't expect any errors here so 
			// if we get one here, that's an error
			// on the sending side.
			if err != nil {
				return err
			}

			var ok bool

			line, ok = v.(cacheLine)
			if !ok {
				return fmt.Errorf("broadcast: expected id value to be of type %T, got %T", line, v)
			}

			return nil
		}))

		c.l.Unlock()  // allow data to be written
		wait.Lock()   // block until the closure above is run
		unreg()       // unregister the handler
		wait.Unlock() // clean up and prepare for another iteration
		c.l.Lock()    // take the global lock again

		// funny construction ends here

		data.IDs[line.id] = line.ms 
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)

	// this only prints an error if there was one
	wrapError("error encoding json", enc.Encode(data), 500).ServeHTTP(w, r)
}

