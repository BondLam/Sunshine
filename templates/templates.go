// Package templates 内嵌 Sunshine 的工程骨架模板（随二进制发布）。
package templates

import "embed"

// Sunshine 是 .sunshine/ 工程骨架的内嵌模板文件系统。
//
//go:embed all:sunshine
var Sunshine embed.FS

// Platforms 是三平台（Claude Code、Cursor、Codex）的内嵌模板文件系统，
// 由 internal/platforms 在 Install 时按平台渲染写出。
// templates/platforms/common/bundled-skills/ 由 B 组另行翻译写入；
// 该目录缺失时 Install 跳过内置多文件 skill，不报错。
//
//go:embed all:platforms
var Platforms embed.FS

// Workflows 是三个工作流模板（native、tdd、
// channel-driven-subagent-dispatch）的中文翻译版，
// 由 internal/workflow 在 `sunshine workflow` 切换时写出。
//
//go:embed all:workflows
var Workflows embed.FS

// Design 是产品设计文档模板（需求列表、概要设计、原型），
// 只在 init 选“产品设计”时由 internal/scaffold 写进项目 doc/，
// 不在 .sunshine/ 里，也不在模板哈希清单里。
//
//go:embed all:design
var Design embed.FS
