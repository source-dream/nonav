package httpserver

import "errors"

var (
	errNotFound     = errors.New("not found")
	errUnauthorized = errors.New("unauthorized")
)
