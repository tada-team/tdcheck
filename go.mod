module github.com/tada-team/tdcheck

go 1.16

replace github.com/tada-team/tdclient => ../tdclient

require (
	github.com/dustin/go-humanize v1.0.0
	github.com/gorilla/mux v1.8.0
	github.com/pkg/errors v0.9.1
	github.com/tada-team/kozma v1.1.0
	github.com/tada-team/tdclient v0.7.0
	github.com/tada-team/tdproto v1.49.12
	gopkg.in/yaml.v2 v2.3.0
)
