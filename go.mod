module github.com/tada-team/tdcheck

go 1.14

require (
	github.com/gorilla/mux v1.8.0
	github.com/pkg/errors v0.9.1
	github.com/satori/go.uuid v1.2.0 // indirect
	github.com/tada-team/kozma v1.0.3
	github.com/tada-team/tdclient v0.1.0
	gopkg.in/yaml.v2 v2.3.0
)

//replace github.com/tada-team/tdclient v0.0.0 => ../tdclient
