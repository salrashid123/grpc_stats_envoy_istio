module main

go 1.14

require (
	contrib.go.opencensus.io/exporter/prometheus v0.2.0 // indirect
	github.com/google/uuid v1.1.2 // indirect
	github.com/prometheus/client_golang v1.9.0 // indirect
	golang.org/x/net v0.0.0-20200822124328-c89045814202 // indirect
	google.golang.org/grpc v1.31.1 // indirect
	google.golang.org/protobuf v1.25.0 // indirect
	helloworld v0.0.0
)

replace helloworld => ./helloworld
