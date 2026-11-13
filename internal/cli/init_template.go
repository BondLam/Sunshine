package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/linrong007/Sunshine/internal/registry"
)

// 交互选模板的两个固定选项（标签即选单里显示的文字）。
const (
	// blankTemplateLabel 从零开始：用内置空白规范模板（上游的 from scratch）。
	blankTemplateLabel = "从零开始（默认）"
	// customTemplateLabel 自定义：输入一个模板市场来源再选。
	customTemplateLabel = "自定义（输入一个模板市场来源）"
)

// promptSpecTemplate 弹出规范模板选单（默认实现用 survey；测试可替换）。
// options 是选单里显示的标签，def 是默认选中的标签。
var promptSpecTemplate = func(options []string, def string) (string, error) {
	var chosen string
	err := survey.AskOne(&survey.Select{
		Message: "请选择规范模板：",
		Options: options,
		Default: def,
	}, &chosen)
	return chosen, err
}

// promptCustomRegistrySource 输入自定义模板市场来源（默认实现用 survey；
// 测试可替换）。返回空字符串表示返回上一步。
var promptCustomRegistrySource = func() (string, error) {
	var source string
	err := survey.AskOne(&survey.Input{
		Message: "请输入模板市场来源（如 gh:owner/repo、git URL 或本地路径），直接回车返回上一步：",
	}, &source)
	return strings.TrimSpace(source), err
}

// templateChoice 是选单里的一项：显示标签与对应的模板 id。
// kind 为 blank（从零开始）或 custom（自定义来源）时 id 为空。
type templateChoice struct {
	label string
	id    string
	kind  string
}

// specTemplateChoices 把市场索引里的条目转成选单选项：只列 spec 型模板
// （与上游 init.ts 的 filter 一致），标签为“id（name）”，无 name 时只写 id。
func specTemplateChoices(templates []registry.SpecTemplate) []templateChoice {
	var choices []templateChoice
	for _, t := range templates {
		if t.Type != "spec" {
			continue
		}
		label := t.ID
		if strings.TrimSpace(t.Name) != "" {
			label = fmt.Sprintf("%s（%s）", t.ID, t.Name)
		}
		choices = append(choices, templateChoice{label: label, id: t.ID, kind: "template"})
	}
	return choices
}

// templateChoiceLabels 取选项的显示标签（选单用）。
func templateChoiceLabels(choices []templateChoice) []string {
	labels := make([]string, 0, len(choices))
	for _, c := range choices {
		labels = append(labels, c.label)
	}
	return labels
}

// templateChoiceID 按显示标签找回模板 id（选单返回的是标签）。
func templateChoiceID(choices []templateChoice, label string) string {
	for _, c := range choices {
		if c.label == label {
			return c.id
		}
	}
	return ""
}

// resolveInteractiveTemplate 在交互模式且未传 --template 时，按上游 init.ts
// 的交互选模板流程确定 spec 模板来源：
//
//   - 已传 --registry：探测到市场就列出其中的 spec 模板供选；
//     无 index.json 则走直接下载；探测瞬时失败直接报错（与上游一致）；
//   - 未传 --registry：选单为“从零开始（默认）”+“自定义（输入来源）”，
//     自定义来源探测到市场时再列出其中的 spec 模板，
//     无 index.json 则走直接下载，来源无效或探测失败时打印原因后回选单。
//
// 返回最终来源、模板 id 与解析好的 Target（从零开始时三者皆空）。
// Target 已接管探测资源，调用方负责 Close（安装后再次 Close 是安全的）。
// -y 与非交互终端不走这里（调用方保证）。
func resolveInteractiveTemplate(registrySource string) (source, templateID string, target *registry.Target, err error) {
	source = registrySource
	for {
		if source == "" {
			chosen, aerr := promptSpecTemplate([]string{blankTemplateLabel, customTemplateLabel, quitOptionLabel}, blankTemplateLabel)
			if aerr != nil {
				return "", "", nil, fmt.Errorf("选择规范模板失败：%w", aerr)
			}
			if chosen == quitOptionLabel {
				return "", "", nil, errInitCancelled
			}
			if chosen == blankTemplateLabel {
				return "", "", nil, nil
			}
			input, aerr := promptCustomRegistrySource()
			if aerr != nil {
				return "", "", nil, fmt.Errorf("输入模板市场来源失败：%w", aerr)
			}
			if input == "" {
				continue
			}
			source = input
		}
		// 来源是 --registry 显式给的（而非本轮自定义输入的）时，
		// 来源无效或探测失败直接报错退出，与上游一致；自定义输入的
		// 则打印原因后回选单重选。
		fromFlag := registrySource != "" && source == registrySource
		src, perr := registry.ParseRegistrySource(source)
		if perr != nil {
			if fromFlag {
				return "", "", nil, perr
			}
			fmt.Printf("模板市场来源无效：%v\n", perr)
			source = ""
			continue
		}
		fmt.Printf("正在拉取模板列表：%s……\n", source)
		probe := registry.ProbeRegistryIndex(context.Background(), src)
		if probe.Err != nil {
			probe.Close()
			if fromFlag {
				return "", "", nil, probe.Err
			}
			fmt.Printf("无法访问模板市场：%v，请重试或换一个来源。\n", probe.Err)
			source = ""
			continue
		}
		if probe.IsNotFound {
			// 直接下载模式：整个来源内容装进 .sunshine/spec。
			return source, "", &registry.Target{Source: src, Mode: "direct", Probe: probe}, nil
		}
		choices := specTemplateChoices(probe.Templates)
		if len(choices) == 0 {
			probe.Close()
			fmt.Println("该模板市场里没有规范模板，改用内置空白规范模板。")
			return "", "", nil, nil
		}
		labels := append(templateChoiceLabels(choices), quitOptionLabel)
		chosen, aerr := promptSpecTemplate(labels, labels[0])
		if aerr != nil {
			probe.Close()
			return "", "", nil, fmt.Errorf("选择规范模板失败：%w", aerr)
		}
		if chosen == quitOptionLabel {
			probe.Close()
			return "", "", nil, errInitCancelled
		}
		id := templateChoiceID(choices, chosen)
		t, terr := registry.ResolveTargetWithProbe(src, probe, id)
		if terr != nil {
			return "", "", nil, terr
		}
		return source, id, t, nil
	}
}
