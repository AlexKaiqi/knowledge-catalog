package server

import (
	"net/http"

	objectserving "kc/liveaccess"
)

// HTTPStateLookup 与 NewHTTPStateLookup 是公开面保持：origin 绑定查找的
// 实现已迁 kc/object/serving（通用合同件，invocation 的 local 组合也
// 需要），server 侧经别名继续暴露，行为不变。
type HTTPStateLookup = objectserving.HTTPStateLookup

// NewHTTPStateLookup builds a generic HTTP client. The call target comes from
// the pinned Schema origin on each request.
func NewHTTPStateLookup(client *http.Client) *objectserving.HTTPStateLookup {
	return objectserving.NewHTTPStateLookup(client)
}
