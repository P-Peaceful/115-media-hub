// generate.go：声明从共享 OpenAPI 合同生成 Go 传输类型的固定命令；生成结果由工具维护。

// Package generated 提供共享 API 合同生成的传输类型。
package generated

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.2 --config ../../../oapi-codegen.yaml ../../../../api/openapi.yaml
