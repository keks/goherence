package goherence

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type logLocker struct {
	l    sync.Locker
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

func millis(t time.Time) int64 {
	return t.UnixNano() / 1000000
}

type writerToFunc func(w io.Writer) (int64, error)

func (wt writerToFunc) WriteTo(w io.Writer) (int64, error) {
	return wt(w)
}

type countWriter struct {
	n int64
	w io.Writer
}

func (cw *countWriter) Write(data []byte) (int, error) {
	n, err := cw.w.Write(data)
	cw.n += int64(n)

	return n, err
}
