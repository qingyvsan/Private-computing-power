package v1

import (
	"encoding/json"
	"fmt"

	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/mem"
)

// 由于本项目使用手写的 Go 结构体（未使用 protoc 生成），
// 无法实现 proto.Message 接口。这里注册一个 JSON codec，
// 使 gRPC 使用 JSON 进行序列化。
//
// 结构体中的 json tag 与 proto 字段名保持一致，
// 因此 wire 格式仍遵循 proto3 JSON 映射规范。

const jsonName = "proto-json"

func init() {
	encoding.RegisterCodecV2(JSONCodec{})
}

// JSONCodec 基于 encoding/json 的 gRPC codec
type JSONCodec struct{}

func (JSONCodec) Name() string { return jsonName }

func (JSONCodec) Marshal(v any) (mem.BufferSlice, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("json marshal: %w", err)
	}
	return mem.BufferSlice{mem.NewBuffer(&data, nil)}, nil
}

func (JSONCodec) Unmarshal(data mem.BufferSlice, v any) error {
	if err := json.Unmarshal(data.Materialize(), v); err != nil {
		return fmt.Errorf("json unmarshal: %w", err)
	}
	return nil
}
