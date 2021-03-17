package checkers

import "net/http"

type Checker interface {
	Enabled() bool
	Start()
	Report(w http.ResponseWriter)
}
