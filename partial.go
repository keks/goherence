package goherence

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net/http"
	"sync"
	"time"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"

	"github.com/keks/marx"
)

type Partial interface {
	ID() string
	Href() string
	Timestamp() time.Time

	http.Handler
	HTML() template.HTML
}

type StaticPartial struct {
	Value interface{}

	id, href string
	ts time.Time
	rf RenderFunc
}

func (sp StaticPartial) ID() string {return sp.id}
func (sp StaticPartial) Href() string {return sp.href}
func (sp StaticPartial) Timestamp() time.Time {return sp.ts}

func (sp StaticPartial) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	writerToHandler(sp.rf(sp.Value, sp)).ServeHTTP(w, r)
}

func (sp StaticPartial) HTML() template.HTML {
	var buf bytes.Buffer
	_, err := sp.rf(sp.Value, sp).WriteTo(&buf)
	if err != nil {
		return template.HTML(fmt.Sprintf("render error: %s", err))
	}

	return template.HTML(buf.String())
}

type ObservablePartial struct {
	Observable luigi.Observable

	id string
	time time.Time
	rf RenderFunc
	l sync.Mutex
	v interface{}
}

func NewObservablePartial(id string, rf RenderFunc, obv luigi.Observable) *ObservablePartial {
	op := &ObservablePartial{
		id: id,	
		rf: rf,
		Observable: obv,
		time: time.Now(),
	}

	op.Observable.Register(
		luigi.FuncSink(
			func(ctx context.Context, v interface{}, err error) error {
				op.l.Lock()
				defer op.l.Unlock()

				op.time = time.Now()
				op.v = v

				return nil
	}))

	return op
}

func (op *ObservablePartial) handler() http.Handler {
	return writerToHandler(op.rf(op.v, op))
}

func (op *ObservablePartial) ID() string {return op.id}
func (op *ObservablePartial) Href() string {
	return fmt.Sprintf("/%s", op.id)
}
func (op *ObservablePartial) Timestamp() time.Time {return op.time}

func (op *ObservablePartial) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	op.handler().ServeHTTP(w, r)
}

func (op *ObservablePartial) HTML() template.HTML {
	var buf bytes.Buffer
	_, err := op.rf(op.v, op).WriteTo(&buf)
	if err != nil {
		return template.HTML(fmt.Sprintf("render error: %s", err))
	}

	return template.HTML(buf.String())
}

func NewLogPartial(id string, rf RenderFunc, log margaret.Log) (*LogPartial, marx.Worker, error) {
	lp := &LogPartial{
		id: id,	
		rf: rf,
		Log: log,
		time: time.Now(),
	}

	iseq, err := log.Seq().Value()
	if err != nil {
		return nil, nil, err
	}

	seq := iseq.(margaret.Seq)
	src, err := log.Query(margaret.Live(true), margaret.Gt(seq), margaret.SeqWrap(true))
	if err != nil {
		return nil, nil, err
	}

	sink := luigi.FuncSink(func(ctx context.Context, v interface{}, err error) error {
		lp.time = time.Now()
		sw := v.(margaret.SeqWrapper)

		lp.l.Lock()
		defer lp.l.Unlock()

		lp.v = sw.Value()
		lp.seq = sw.Seq()

		return nil
	})

	worker := marx.Worker(func(ctx context.Context) error {
		return luigi.Pump(ctx, sink, src)
	})

	return lp, worker, nil
}

type LogPartial struct {
	Log margaret.Log

	id string
	time time.Time
	rf RenderFunc

	l sync.Mutex
	v interface{}
	seq margaret.Seq
}

func (lp *LogPartial) handler(since int64) http.Handler {
	fmt.Println("LOG get since", since)
	src, err := lp.Log.Query(margaret.Gt(margaret.BaseSeq(since)))
	if err != nil {
		return newError("error querying log", err, 500)
	}

	return lp.streamHandler(src, lp.rf, since)
}

// streamHandler returns an http handler that writes the passed stream.
func (lp *LogPartial) streamHandler(src luigi.Source, rf RenderFunc, since int64) http.Handler {
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
		return writerToHandler(rf(v, lp)), true
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
		fmt.Fprintf(w, fmtStr, lp.id, lp.id, since, millis(lp.time))
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

func (lp *LogPartial) ID() string {return lp.id}
func (lp *LogPartial) Href() string {
	// TODO put the sequence number here
	return fmt.Sprintf("/%d?since=TODO", lp.id)
}
func (lp *LogPartial) Timestamp() time.Time {return lp.time}

func (lp *LogPartial) HTML() template.HTML {
	str := func() string {
		var (
			buf bytes.Buffer
			since margaret.Seq = margaret.SeqEmpty
		)

		src, err := lp.Log.Query(
			margaret.Gt(margaret.BaseSeq(-1)),
			margaret.Live(false),
			margaret.SeqWrap(true))
		if err != nil {
			return fmt.Sprint("error querying log:", err)
		}

		for {
			v, err := src.Next(context.TODO())
			if luigi.IsEOS(err) {
				break
			} else if err != nil {
				fmt.Fprint(&buf, "error reading log:", err)
				return buf.String()
			}

			sw := v.(margaret.SeqWrapper)
			v = sw.Value()
			since = sw.Seq()

			_, err = lp.rf(v, lp).WriteTo(&buf)
			if err != nil {
				fmt.Fprint(&buf, "render error:", err)
				return buf.String()
			}
		}

		fmtStr := `<div data-id="%s-latest" data-href="/%s?since=%d" data-ts="%d"></div>`
		fmt.Fprintf(&buf, fmtStr, lp.id, lp.id, since.Seq(), millis(lp.time))

		return buf.String()
	}()

	return template.HTML(str)
}
