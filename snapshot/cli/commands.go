package cli

import "slices"

var commandEntries = []Command{
	{Path: "show", Groups: []string{"composition", "dataset"}, Handler: "show", Action: "inventory.read", Description: "显示服务的 Repository 与 Dataset 库存", Usage: helpShow},
	{Path: "attach", Groups: []string{"composition"}, Operands: []string{"repo"}, Handler: "attach", Action: "repositories.manage", Description: "把已连接 Repository 挂入服务登记空间", Usage: helpAttach},
	{Path: "detach", Groups: []string{"composition"}, Operands: []string{"repo"}, Handler: "detach", Action: "repositories.manage", Description: "从服务登记空间移除 Repository 登记", Usage: helpDetach},
	{Path: "grant add", Groups: []string{"composition", "dataset", "grant"}, Handler: "grant-add", Action: "admin.grants.manage", Description: "发放一条服务、仓或 Dataset 范围的授权规则", Usage: helpGrantAdd},
	{Path: "grant list", Groups: []string{"composition", "dataset", "grant"}, Handler: "grant-list", Action: "admin.grants.read", Description: "列出授权规则", Usage: helpGrantList},
	{Path: "grant remove", Groups: []string{"composition", "dataset", "grant"}, Handler: "grant-remove", Action: "admin.grants.manage", Description: "撤销一条授权规则", Usage: helpGrantRemove},
	{Path: "grant update", Groups: []string{"composition", "dataset", "grant"}, Handler: "grant-update", Action: "admin.grants.manage", Description: "按此前观察的旧规则原子修改授权，保持规则身份与来源", Usage: helpGrantUpdate, Flags: []string{"server", "as", "request-id", "trace-id", "span-id", "parent-span-id", "help", "id", "file", "expected-file"}},
	{Path: "registry audit", Groups: []string{"composition", "registry"}, Handler: "registry-audit", Action: "registry.audit.read", Description: "查看服务登记历史", Usage: helpRegistryAudit},
	{Path: "dataset define", Groups: []string{"composition", "dataset"}, Operands: []string{"dataset"}, Handler: "dataset-define", Action: "dataset.manage", Description: "发布 Dataset 的固定版本文件交付范围（目录映射与逐文件交付）", Usage: helpDatasetDefine},
	{Path: "dataset show", Groups: []string{"composition", "dataset", "files"}, Operands: []string{"dataset"}, Handler: "dataset-show", Action: "inventory.read", Description: "查看 Dataset 已接受发布的定义、来源、元数据与一页历史", Usage: helpDatasetShow, Flags: []string{"server", "as", "request-id", "trace-id", "span-id", "parent-span-id", "help", "dataset", "revision", "limit", "continuation"}},
	{Path: "files list", Groups: []string{"composition", "dataset", "files"}, Handler: "files-list", Action: "file.read", Description: "分页列出 Dataset 交付目录的直接子项", Usage: helpFilesList, Flags: []string{"server", "as", "request-id", "trace-id", "span-id", "parent-span-id", "help", "dataset", "revision", "path", "limit", "continuation"}},
	{Path: "files read", Groups: []string{"composition", "dataset", "files"}, Handler: "files-read", Action: "file.read", Description: "读取仓中固定版本或 Dataset 交付的文件字节，或下载完整单文件", Usage: helpFilesRead, Flags: []string{"server", "as", "request-id", "trace-id", "span-id", "parent-span-id", "help", "repo", "commit", "dataset", "revision", "path", "offset", "length", "out"}},
	{Path: "dataset retire", Groups: []string{"composition", "dataset"}, Operands: []string{"dataset"}, Handler: "dataset-retire", Action: "dataset.manage", Description: "退役 Dataset，停止消费并保留发布记录", Usage: helpDatasetRetire},
	{Path: "dataset overlay", Groups: []string{"composition", "dataset"}, Handler: "dataset-overlay", Action: "dataset.overlay", Description: "只在本机合成一份不发表的配方", Usage: helpDatasetOverlay, Flags: []string{"file", "overlay", "out", "help"}},
}

func Commands() []Command {
	out := slices.Clone(commandEntries)
	for i := range out {
		out[i].Flags = slices.Clone(out[i].Flags)
		out[i].Operands = slices.Clone(out[i].Operands)
		out[i].Groups = slices.Clone(out[i].Groups)
	}
	return out
}

func hasCommand(path string) bool {
	for _, entry := range commandEntries {
		if entry.Path == path {
			return true
		}
	}
	return false
}

func commandFlags(path string) map[string]struct{} {
	for _, entry := range commandEntries {
		if entry.Path == path && len(entry.Flags) > 0 {
			result := map[string]struct{}{}
			for _, name := range entry.Flags {
				result[name] = struct{}{}
			}
			return result
		}
	}
	return nil
}
