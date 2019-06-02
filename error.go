package goherence

import (
	"fmt"
	"net/http"
)

type Error struct {
	Message string
	Cause   error
	Status  int
}

func wrapError(msg string, cause error, status int) *Error {
	if cause == nil {
		return nil
	}

	return newError(msg, cause, status)
}

func newError(msg string, cause error, status int) *Error {
	return &Error{
		Message: msg,
		Cause:   cause,
		Status:  status,
	}
}

func (err *Error) Unwrap() error {
	return err.Cause
}

func (err *Error) Error() string {
	if err.Cause != nil {
		return fmt.Sprintf("%s: %s", err.Message, err.Cause)
	}

	return err.Message
}

func (err *Error) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err == nil {
		return
	}

	http.Error(w, err.Error(), err.Status)
}
