// settings.json 的 hook 合并：sunshine update 冲突覆盖
// .claude/settings.json 时，只补模板里有、用户文件里缺失的 hook 组
// （如新增的 SubagentStop），用户已有配置一行不动。
//
// 背景：update 的三态里，自动更新（磁盘未改）直接写模板全文；
// 冲突覆盖旧行为也是模板全文，会丢掉用户自加的 hook/env 等定制。
// v0.5.0 起 settings.json 模板新增 SubagentStop，demo 这类已有项目
// 很可能已定制过 settings.json，整文件覆盖不安全，
// 因此覆盖时走本合并：按 command 去重补缺失组。
package platforms

import (
	"fmt"
)

// MergeMissingSettingsHooks 把 fresh 模板 settings.json 里缺失的 hook
// 组补进 existing（用户已有配置），返回合并后的内容（2 空格缩进，
// 末尾换行）。
//   - 顶层键顺序与已有值全部保留；新键追加到末尾。
//   - hooks 下每个事件：已有 matcher 组按 command 去重，fresh 里有、
//     existing 里没有的组追加到该事件末尾；existing 没有该事件时，
//     整个事件取 fresh 的值。
//   - 任一解析失败返回错误（调用方回落到模板全文覆盖）。
func MergeMissingSettingsHooks(existing, fresh []byte) ([]byte, error) {
	ex, err := parseOrderedJSON(existing)
	if err != nil {
		return nil, fmt.Errorf("解析已有 settings.json 失败：%w", err)
	}
	fr, err := parseOrderedJSON(fresh)
	if err != nil {
		return nil, fmt.Errorf("解析模板 settings.json 失败：%w", err)
	}
	freshHooksRaw, _ := fr.get("hooks")
	freshHooks, _ := freshHooksRaw.(*orderedObject)
	if freshHooks == nil {
		return existing, nil
	}
	exHooksRaw, _ := ex.get("hooks")
	exHooks, _ := exHooksRaw.(*orderedObject)
	if exHooks == nil {
		exHooks = &orderedObject{vals: make(map[string]any)}
		ex.set("hooks", exHooks)
	}
	for _, event := range freshHooks.keys {
		freshGroups, _ := freshHooks.vals[event].([]any)
		if freshGroups == nil {
			continue
		}
		raw, ok := exHooks.vals[event]
		if ok {
			if _, isArr := raw.([]any); !isArr {
				continue
			}
		}
		exGroups, _ := raw.([]any)
		have := map[string]bool{}
		for _, g := range exGroups {
			if cmd := hookGroupCommand(g); cmd != "" {
				have[cmd] = true
			}
		}
		changed := false
		for _, g := range freshGroups {
			cmd := hookGroupCommand(g)
			if cmd == "" || have[cmd] {
				continue
			}
			exGroups = append(exGroups, g)
			have[cmd] = true
			changed = true
		}
		if changed {
			exHooks.set(event, exGroups)
		}
	}
	out, err := ex.marshalIndent()
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// hookGroupCommand 取一个 matcher 组里的 hook command（第一个）。
// 组形状：{"hooks": [{"command": "...", ...}], "matcher": "..."}。
func hookGroupCommand(g any) string {
	obj, ok := g.(*orderedObject)
	if !ok {
		return ""
	}
	hooksRaw, _ := obj.get("hooks")
	hooks, _ := hooksRaw.([]any)
	for _, h := range hooks {
		if ho, ok := h.(*orderedObject); ok {
			if cmdRaw, _ := ho.get("command"); cmdRaw != nil {
				if cmd, _ := cmdRaw.(string); cmd != "" {
					return cmd
				}
			}
		}
	}
	return ""
}
