package goherence

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
)

type Partial interface {
	ID() string
	Href() string
	Timestamp() time.Time
	
	http.Handler
}

type ObservablePartial struct {
	Observable luigi.Observable

	id string
	time time.Time
	rf RenderFunc2
}

func (op *ObservablePartial) handler() http.Handler {
	v, err := op.Observable.Value()
	if err != nil {
		return newError("error getting value of observable", err, 500)
	}

	return op.rf(v)
}

func (op *ObservablePartial) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	op.handler().ServeHTTP(w, r)
}

type LogPartial struct {
	Log margaret.Log

	id string
	time time.Time
	rf RenderFunc2
}

func (lp *LogPartial) handler(since int64) http.Handler {
	src, err := lp.Log.Query(margaret.Gt(margaret.BaseSeq(since)))
	if err != nil {
		return newError("error querying log", err, 500)
	}

	return lp.streamHandler(src, lp.rf, since)
}

// streamHandler returns an http handler that writes the passed stream.
func (lp *LogPartial) streamHandler(src luigi.Source, rf RenderFunc2, since int64) http.Handler {
	// returns handler that reads and processeses individual item in stream
	// the second return value indicates whether more data is available.
	itemHandler := func(ctx context.Context) (http.Handler, bool) {
		v, err := src.Next(ctx)
		if luigi.IsEOS(err) {
			return lp.latestHandler(since), false
		} else if err != nil {
			return newError("error reading stream", err, 500), false
		}

		since++
		return rf(v), true
	}

	// return the handler that iterates over the stream
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var (
			h http.Handler
			more bool = true
			ctx = r.Context()
		)

		for more {
			h, more = itemHandler(ctx)
			h.ServeHTTP(w, r)
		}
	})
}

// latestHandler returns an http handler that prints the latest-div (for autoupdates)
func (lp *LogPartial) latestHandler(since int64) http.Handler {
	fmtStr := `<div data-id="%s-latest" data-href="/%s?since=%d" data-ts="%d"></div>`
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, fmtStr, lp.id, lp.id, since, lp.time)
	})
}

func (lp *LogPartial) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var h http.Handler

	since, err := getSince(r)
	if err != nil {
		h = err
	} else {
		h = lp.handler(since)
	}

	h.ServeHTTP(w, r)
}
