module github.com/tada-team/tdcheck

go 1.14

require (
	github.com/dustin/go-humanize v1.0.0
	github.com/gorilla/mux v1.8.0
	github.com/pion/webrtc/v2 v2.2.26
	github.com/pkg/errors v0.9.1
	github.com/tada-team/kozma v1.1.0
	github.com/tada-team/tdclient v0.6.4
	github.com/tada-team/tdproto v1.25.1
	gopkg.in/yaml.v2 v2.3.0
)

//replace github.com/tada-team/tdclient v0.6.3 => ../tdclient
