package cli_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	snapshotcli "kc/snapshot/cli"
	"kc/snapshot/client"
)

func TestSnapshotLeafHelpExplainsTaskConditionsEffectsAndRecovery(t *testing.T) {
	sections := []string{"用途：", "用法与参数：", "条件：", "结果：", "后果：", "风险与成本：", "失败恢复：", "示例："}
	for _, command := range snapshotcli.Commands() {
		t.Run(command.Path, func(t *testing.T) {
			if !strings.HasPrefix(command.Usage, "kc "+command.Path) {
				t.Fatal("leaf usage must start with its public invocation")
			}
			previous := -1
			for i, section := range sections {
				at := strings.Index(command.Usage, section)
				if at <= previous {
					t.Fatalf("task help is missing or misorders %s", section)
				}
				end := len(command.Usage)
				if i+1 < len(sections) {
					end = strings.Index(command.Usage, sections[i+1])
				}
				if end <= at+len(section) || strings.TrimSpace(command.Usage[at+len(section):end]) == "" {
					t.Fatalf("task help has no explanation for %s", section)
				}
				previous = at
			}
			if !strings.Contains(command.Usage, "kc "+command.Path) {
				t.Fatal("task help lacks its own executable command")
			}
		})
	}
}

func TestSnapshotHelpPreservesDistinctWriteAndConsumptionGuarantees(t *testing.T) {
	claims := map[string][]string{
		"show":            {"inventory.read", "库存可见不表示", "结果规模"},
		"attach":          {"不创建仓", "不隐含发权", "同一 --repo 重试"},
		"detach":          {"保留来源文件", "旧版本坐标不能绕过", "误移除"},
		"registry audit":  {"registry.audit.read", "当前入口不返回续页游标", "不是完整历史"},
		"dataset define":  {"可在发布前", "recipeSkipped", "不回滚已完成的配方写回", "NON_FAST_FORWARD"},
		"dataset show":    {"servingRevision", "单页不是全部历史", "游标失效"},
		"dataset retire":  {"没有取消退役", "同名 Dataset", "保留发布记录"},
		"dataset overlay": {"无需登录", "会覆盖已有文件", "不提供原子替换", "不能解释成已接受发布版"},
		"files list":      {"续页必须显式沿用返回的 revision", "空页带 continuation", "不承诺来源端"},
		"files read":      {"base64", "硬链接发布", "进程被强制终止", "按原 revision 重试"},
		"grant add":       {"重复提交不是幂等", "省略资源范围", "不能盲目重发"},
		"grant list":      {"全局 admin.grants.read", "不是完整有效权限集合", "不包含部署默认消费声明"},
		"grant remove":    {"其它规则", "已交付", "失去后续管理权"},
		"grant update":    {"省略的限定会清空", "旧规则和新范围", "保留旧规则", "不能自动刷新后盲目覆盖"},
	}
	for _, command := range snapshotcli.Commands() {
		for _, claim := range claims[command.Path] {
			if !strings.Contains(command.Usage, claim) {
				t.Errorf("%s help lost its distinct guarantee or recovery advice %q", command.Path, claim)
			}
		}
		delete(claims, command.Path)
	}
	if len(claims) != 0 {
		t.Fatalf("help claims refer to absent commands: %v", claims)
	}
}

func TestEverySnapshotLeafHelpUsesStaticDeclarationWithoutEffects(t *testing.T) {
	transport := &testTransport{client: http.DefaultClient}
	sdk, err := client.New(client.Config{BaseURL: "http://unused.invalid", Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	recipe, overlay, output := filepath.Join(dir, "recipe.yaml"), filepath.Join(dir, "overlay.yaml"), filepath.Join(dir, "output.yaml")
	for path, body := range map[string]string{
		recipe:  "name: help-test\nmounts:\n- repository: source\n  selector: published\n  path: reference\n",
		overlay: "mounts:\n- repository: other\n  selector: published\n  path: extra\n",
		output:  "preserve this target\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, command := range snapshotcli.Commands() {
		flags := map[string]any{"help": true}
		if command.Path == "dataset overlay" {
			flags["file"], flags["overlay"], flags["out"] = recipe, overlay, output
		}
		value, err := snapshotcli.New(sdk, snapshotcli.Context{}).Execute(context.Background(), command.Path, flags, client.RequestOptions{})
		if err != nil || value != command.Description+"\n\n"+command.Usage {
			t.Fatalf("%s help did not return its static declaration: %#v %v", command.Path, value, err)
		}
	}
	if transport.calls != 0 {
		t.Fatalf("leaf help contacted HTTP %d times", transport.calls)
	}
	if body, err := os.ReadFile(output); err != nil || string(body) != "preserve this target\n" {
		t.Fatalf("overlay help wrote the existing target: %q %v", body, err)
	}
}
