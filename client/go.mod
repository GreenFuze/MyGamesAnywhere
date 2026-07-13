module github.com/GreenFuze/MyGamesAnywhere/client

go 1.25.0

require (
	github.com/GreenFuze/MyGamesAnywhere/protocol v0.0.0
	github.com/coder/websocket v1.8.15
	github.com/google/uuid v1.6.0
	github.com/spf13/cobra v1.10.2
	golang.org/x/sys v0.47.0
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
)

replace github.com/GreenFuze/MyGamesAnywhere/protocol => ../protocol
