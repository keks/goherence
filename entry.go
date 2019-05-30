package goherence

import (
	"fmt"
	"time"

	"go.cryptoscope.co/luigi"
)

type Entry struct {
	ID string
	Observable luigi.Observable
	Time  time.Time
}

func (e *Entry) Value() (v_ string) {
	defer func() {
		fmt.Println("e.Value returned", v_)
	}()

	if e.Observable == nil {
		return "[nil]"
	}

	v, err := e.Observable.Value()
	if err != nil {
		return fmt.Sprint("error reading observable value:", err)
	}

	return fmt.Sprint(v)
}

