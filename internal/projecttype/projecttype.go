// Package projecttype 检测单仓项目的类型（前端 / 后端 / 全栈 / 未知），
// 并解析 --type 的手动指定值。
//
// 检测只看项目根目录的文件信号与 package.json 的依赖名：
// 前端信号与后端信号都有 → 全栈；只有一侧 → 对应类型；都没有 → 未知。
// 与上游行为有一处有意偏离：package.json 存在本身不算前端信号，
// 只有命中前端依赖才算——否则纯 Express 后端会被误判为全栈。
// 文件不存在、读失败、package.json 解析失败一律当作无信号，不报错。
package projecttype

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Type 是单仓项目的类型。
type Type int

const (
	// Unknown 表示检测不到任何信号（零值）。
	Unknown Type = iota
	// Frontend 表示前端项目。
	Frontend
	// Backend 表示后端项目。
	Backend
	// Fullstack 表示前后端全栈项目。
	Fullstack
)

// String 返回类型的英文名，供命令行与日志使用。
func (t Type) String() string {
	switch t {
	case Frontend:
		return "frontend"
	case Backend:
		return "backend"
	case Fullstack:
		return "fullstack"
	default:
		return "unknown"
	}
}

// DisplayName 返回类型的中文展示名。
func (t Type) DisplayName() string {
	switch t {
	case Frontend:
		return "前端"
	case Backend:
		return "后端"
	case Fullstack:
		return "全栈"
	default:
		return "未知"
	}
}

// Parse 解析 --type 的手动指定值，接受中文名与英文名。
// 其余值（包括 "unknown" 与空串）一律返回错误。
func Parse(s string) (Type, error) {
	switch s {
	case "前端", "frontend":
		return Frontend, nil
	case "后端", "backend":
		return Backend, nil
	case "全栈", "前后端全栈", "fullstack":
		return Fullstack, nil
	default:
		return Unknown, fmt.Errorf("无效的项目类型：%s（合法值：前端、后端、全栈）", s)
	}
}

// frontendFiles 是前端文件信号（相对项目根目录的路径）。
var frontendFiles = []string{
	// 构建工具配置
	"vite.config.ts",
	"vite.config.js",
	"next.config.js",
	"next.config.ts",
	"next.config.mjs",
	"nuxt.config.ts",
	"nuxt.config.js",
	"webpack.config.js",
	"rollup.config.js",
	// 框架配置
	"svelte.config.js",
	"astro.config.mjs",
	"angular.json",
	"vue.config.js",
	// 源码入口
	"src/App.tsx",
	"src/App.jsx",
	"src/App.vue",
	"src/app/page.tsx",
	"app/page.tsx",
	"pages/index.tsx",
	"pages/index.jsx",
}

// backendFiles 是后端文件信号（相对项目根目录的路径）。
// "*.csproj" 与 "*.fsproj" 是根目录通配，按后缀匹配。
var backendFiles = []string{
	// Go
	"go.mod",
	"go.sum",
	// Rust
	"Cargo.toml",
	"Cargo.lock",
	// Python
	"requirements.txt",
	"pyproject.toml",
	"setup.py",
	"Pipfile",
	"poetry.lock",
	// Java/Kotlin
	"pom.xml",
	"build.gradle",
	"build.gradle.kts",
	// Ruby
	"Gemfile",
	// PHP
	"composer.json",
	// .NET（根目录通配）
	"*.csproj",
	"*.fsproj",
	// Elixir
	"mix.exs",
	// Node.js 后端入口
	"server.ts",
	"server.js",
	"src/server.ts",
	"src/index.ts",
}

// frontendDeps 是 package.json 里表示前端的依赖名。
var frontendDeps = []string{
	"react",
	"vue",
	"svelte",
	"angular",
	"@angular/core",
	"next",
	"nuxt",
	"astro",
	"solid-js",
	"preact",
	"lit",
	"@remix-run/react",
}

// backendDeps 是 package.json 里表示后端的依赖名。
var backendDeps = []string{
	"express",
	"fastify",
	"hono",
	"koa",
	"hapi",
	"nest",
	"@nestjs/core",
	"fastapi",
	"flask",
	"django",
}

// Detect 检测 cwd 处单仓项目的类型。
// 文件不存在、读失败一律当作无信号，不报错。
func Detect(cwd string) Type {
	frontend := hasAnyFile(cwd, frontendFiles)
	backend := hasAnyFile(cwd, backendFiles)

	// package.json 存在本身不算前端信号，只看依赖名。
	hasFrontendDep, hasBackendDep := checkPackageJSON(cwd)
	frontend = frontend || hasFrontendDep
	backend = backend || hasBackendDep

	switch {
	case frontend && backend:
		return Fullstack
	case frontend:
		return Frontend
	case backend:
		return Backend
	default:
		return Unknown
	}
}

// hasAnyFile 检查任一信号文件是否存在。带 "*" 的按根目录后缀通配匹配。
func hasAnyFile(cwd string, names []string) bool {
	for _, name := range names {
		if strings.Contains(name, "*") {
			if hasGlobFile(cwd, name) {
				return true
			}
			continue
		}
		if _, err := os.Stat(filepath.Join(cwd, name)); err == nil {
			return true
		}
	}
	return false
}

// hasGlobFile 检查根目录下是否存在匹配 "*.后缀" 的文件。
func hasGlobFile(cwd, pattern string) bool {
	suffix := strings.TrimPrefix(pattern, "*")
	entries, err := os.ReadDir(cwd)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), suffix) {
			return true
		}
	}
	return false
}

// packageJSON 只取 dependencies 与 devDependencies 的键名。
type packageJSON struct {
	Dependencies    map[string]any `json:"dependencies"`
	DevDependencies map[string]any `json:"devDependencies"`
}

// checkPackageJSON 读 package.json 的依赖名，判断前后端依赖信号。
// 文件不存在、读失败、解析失败时当作无依赖信号，不报错。
func checkPackageJSON(cwd string) (frontend, backend bool) {
	data, err := os.ReadFile(filepath.Join(cwd, "package.json"))
	if err != nil {
		return false, false
	}
	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return false, false
	}
	names := make(map[string]struct{}, len(pkg.Dependencies)+len(pkg.DevDependencies))
	for name := range pkg.Dependencies {
		names[name] = struct{}{}
	}
	for name := range pkg.DevDependencies {
		names[name] = struct{}{}
	}
	for _, dep := range frontendDeps {
		if _, ok := names[dep]; ok {
			frontend = true
			break
		}
	}
	for _, dep := range backendDeps {
		if _, ok := names[dep]; ok {
			backend = true
			break
		}
	}
	return frontend, backend
}
