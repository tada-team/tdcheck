module github.com/tada-team/tdcheck

go 1.14

require (
	github.com/gorilla/mux v1.8.0
	github.com/pkg/errors v0.9.1
	github.com/tada-team/kozma v1.1.0
	github.com/tada-team/tdclient v0.5.2
	github.com/tada-team/tdproto v1.16.0
	gopkg.in/yaml.v2 v2.3.0
)

//replace github.com/tada-team/tdclient v0.1.5 => ../tdclient
