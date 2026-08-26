module computing-power/cli

go 1.26

require (
	computing-power/pkg v0.0.0
	computing-power/proto v0.0.0
	github.com/spf13/cobra v1.9.1
	google.golang.org/grpc v1.70.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	golang.org/x/net v0.32.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20241202173237-19429a94021a // indirect
	google.golang.org/protobuf v1.36.4 // indirect
)

replace (
	computing-power/pkg => ../pkg
	computing-power/proto => ../proto
)
