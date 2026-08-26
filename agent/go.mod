module computing-power/agent

go 1.26

require (
	computing-power/pkg v0.0.0
	computing-power/proto v0.0.0
	google.golang.org/grpc v1.70.0
	google.golang.org/protobuf v1.36.4
	gopkg.in/yaml.v3 v3.0.1
)

replace (
	computing-power/pkg => ../pkg
	computing-power/proto => ../proto
)