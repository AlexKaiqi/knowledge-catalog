package cli

import (
	"encoding/json"
	"io"
	"strings"

	objectmodel "kc/object"
	"kc/object/client"
)

// ComparisonCommand contributes the object-reference mode of the host's diff
// command. The host also owns directory publication preview under that name;
// it selects this mode only when the positional object reference is present.
func ComparisonCommand() Command {
	return Command{Path: "diff", Handler: "object-diff", Action: "object.read", Flags: words("from to shape"), Operands: []string{"reference"}, Description: "比较两份固定视图内的本地对象单元", Usage: comparisonUsage}
}

const comparisonUsage = `kc diff <urn> --from '<fixed-view-json>' --to '<fixed-view-json>' [--shape <selection>]
  两侧分别指定一种固定视图：{"basis":"rb1_..."}、{"repository":"kr://example/orders","commit":"c1"}，或 {"dataset":"orders","revision":1}。
  每侧重新校验当前权限；Dataset 消费权可比较发布内容，不授予来源历史权。
  保留双侧范围与固定坐标。比较所选完整 Unit 的本地值、声明及构成，不调用外部取值。
  字段投影仍比较所属完整 Unit；跨范围不给整体相等结论，切片中的消失只证明离开交付范围。
  任一侧无权、读取失败或组成冲突时失败；不跟随最新版本，不写入。
  例：kc diff urn:kc:object:table/orders --from '{"dataset":"orders","revision":1}' --to '{"dataset":"orders","revision":2}'
  目录发布预览仍使用 kc diff --repo <id> --dir <drafts>，详见 kc help writer commit。`

func comparisonRequest(flags map[string]any) (client.ComparisonRequest, error) {
	request := client.ComparisonRequest{Reference: flagString(flags, "reference"), Shape: flagString(flags, "shape")}
	for _, side := range []struct {
		name string
		view *objectmodel.ComparisonView
	}{{"from", &request.From}, {"to", &request.To}} {
		raw := flagString(flags, side.name)
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(side.view); err != nil {
			return request, usage("--%s requires a fixed-view JSON object", side.name)
		}
		if decoder.Decode(&struct{}{}) != io.EOF {
			return request, usage("--%s requires exactly one fixed-view JSON object", side.name)
		}
		if err := side.view.Validate(); err != nil {
			return request, err
		}
	}
	return request, nil
}
