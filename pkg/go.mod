module computing-power/pkg

go 1.26

require (
	computing-power/proto v0.0.0
	google.golang.org/protobuf v1.36.4
)

require github.com/klauspost/compress v1.19.2 // indirect

replace computing-power/proto => ../proto
