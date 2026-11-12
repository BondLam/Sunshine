---
name: go-sdk-author
description: 设计、构建、验证并发布生产级 Go SDK 与 Go 命令行工具：Go module 布局、公开 API 与模块边界、插件扩展点、错误与版本设计、跨平台构建、GoReleaser + GitHub Release 发布、semver tag 与 alpha/beta/rc/stable 发布生命周期。触发语例如“写一个 Go SDK”、“抽一个 Go 核心库”、“go module 布局”、“公开 API 设计”、“functional options”、“goreleaser 发布”、“semver tag”、“v2 大版本”、“go install 分发”、“Go 库发布流程”、“beta 到 rc”、“Go 静态二进制”。
---

# Go SDK Author（Go SDK 编写）

从零到发布一个生产级 Go SDK / Go 库的端到端工作流：Go module 布局、公开 API 设计、构建配置、分发形态、多包流水线、验证，以及 alpha → beta → rc → stable 的完整发布生命周期。覆盖两种形态：**被别人 import 的库**（`pkg/` / 根包）和**单二进制 CLI**（`cmd/` + GoReleaser 发布）——两者共享同一套 API 与发布纪律。

本文件是统一工作流加日常速查；每个阶段都给出最小可用形态，先跑通再扩展。

---

## 何时使用本 skill

- 从已有 CLI 或应用里抽出核心库（例如 `pkg/core`、`internal/` 提升为可复用的 module）
- 给陌生人用的 Go 库设计公开 API——导出标识符、函数式选项、插件扩展点
- 决定 module 布局：单 module 还是多 module、要不要 `go.work`
- 设计错误处理与版本演进：哨兵错误、`%w` 包装、v2 大版本怎么做
- 配构建与发布：跨平台静态二进制、`go install` 分发、GoReleaser + GitHub Release
- 走发布通道：每提交快照、beta/rc 预发布、stable 发布、发现问题后撤回
- 给库接 CI：`go vet`、`gofmt`、`go test -race`、lint、跨平台编译矩阵

---

## 执行工作流

一个 Go SDK 的构建分七个阶段走。阶段之间有真实依赖，跳过某阶段后面一定会还债。

### 阶段 1——Go Module 与工程布局

先定 module 再写代码。module 名就是别人 import 你的路径，改名成本极高。

核心动作：

1. 一个仓库一个主 module（`go mod init github.com/<组织>/<库名>`）；多二进制/多库的大仓再考虑 `go.work`
2. SDK 本身放 `pkg/<库名>/`（可被外部 import）或仓库根包；只给自己用的代码放 `internal/`
3. CLI 入口放 `cmd/<命令名>/main.go`，每个二进制一个目录
4. 示例放 `examples/`（可编译的示例就是文档），测试与实现放同一目录（`foo.go` / `foo_test.go`）
5. 发布前就想好边界：哪些包是公开契约，哪些是内部实现

```
my-repo/
├── go.mod                # module github.com/acme/sdk
├── go.sum
├── cmd/
│   └── mytool/           # CLI 入口：main.go 只做接线
├── pkg/
│   └── sdk/              # ← 公开库（别人 import 的部分）
│       ├── client.go
│       ├── options.go
│       └── errors.go
├── internal/
│   ├── transport/        # 不对外：HTTP 细节
│   └── state/            # 不对外：内部状态
└── examples/
    └── basic/main.go     # 可编译的用法示例
```

最小可用 `go.mod`：

```go
module github.com/acme/sdk

go 1.24

require (
    // 只留运行时真正依赖的；测试依赖放进 require 块由 go mod tidy 管理
)
```

**原则：** `internal/` 是 Go 的硬边界——`internal/` 下的包只能被以它父目录为根的子树 import，编译器直接拦住。公开契约只放 `pkg/`（或根包），不要靠文档约定“别 import internal”。

### 阶段 2——公开 API 与模块边界

写代码之前先定公开面。陌生人只看到导出标识符，改一个导出名字就是一次破坏性变更。

两个并行关注点：

**A. 模块边界。** `pkg/sdk/` 只导出契约部分，`internal/` 绝不出现在导出签名里：

```
pkg/sdk/
├── client.go     # Client、NewClient、公开方法
├── options.go    # Option、WithXxx（函数式选项）
├── types.go      # 公开类型：ID、请求/响应结构
└── errors.go     # 哨兵错误 ErrNotFound 等
internal/
├── transport/    # HTTP 客户端实现，不导出
└── state/        # 重试、缓存等内部状态
```

**B. API 设计。** 给陌生人用的 API 要满足：

- **函数式选项**：配置用 `Option` 函数收敛，新增选项不破坏旧调用

```go
// NewClient 创建客户端；未传的选项一律用零值/默认值。
func NewClient(addr string, opts ...Option) (*Client, error)

type Option func(*Client)

func WithTimeout(d time.Duration) Option {
    return func(c *Client) { c.timeout = d }
}

func WithTransport(t http.RoundTripper) Option {
    return func(c *Client) { c.transport = t }
}
```

- **接口要小，接收接口、返回具体类型**：参数用最小接口（`io.Reader` 而不是 `*os.File`），返回用具体结构，方便调用方断言和测试替身
- **不透明 ID 用命名类型**：`type UserID string`——字符串换个名字就多一层类型检查，比裸 `string` 安全，且不像 TS 品牌类型那样需要额外技巧
- **错误是值**：哨兵错误 + `%w` 包装，调用方用 `errors.Is` / `errors.As` 判断

```go
var ErrNotFound = errors.New("sdk: 记录不存在")

// 找不到时返回包装后的哨兵错误，调用方可用 errors.Is(err, ErrNotFound) 判断。
func (c *Client) Get(ctx context.Context, id UserID) (*Record, error) {
    rec, err := c.store.get(ctx, id)
    if err != nil {
        return nil, fmt.Errorf("sdk: 读取 %s 失败：%w", id, err)
    }
    return rec, nil
}
```

- **零值可用**：结构体的零值应该是合法状态（空 `Client` 不直接 panic，或文档写明必须用 `NewClient` 创建）
- **可演进**：给结构加字段是兼容的，删字段、改导出函数签名、改接口方法都是破坏性变更；接口一旦公开就很难加方法——优先用结构 + 函数式选项，而不是大接口

### 阶段 3——构建配置

Go 不需要打包器：`go build` 就是构建。关键在编译选项、静态检查与跨平台。

**日常命令：**

```bash
go build ./...            # 全仓编译
go vet ./...              # 官方静态检查（可疑构造、printf 格式、锁复制）
gofmt -l .                # 格式检查：有输出即未格式化
go test ./...             # 全部测试
go test -race ./...       # 竞态检测（并发代码必跑）
go mod tidy               # 同步 go.mod / go.sum，删无用依赖
```

**跨平台静态二进制**（分发给用户的一招）：

```bash
# 纯静态、免 cgo：拿到手就能跑，不依赖目标机器的 libc
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=v0.1.0" -o mytool ./cmd/mytool

# 常用目标：darwin/arm64、darwin/amd64、linux/arm64、windows/amd64
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o mytool-darwin-arm64 ./cmd/mytool
```

- `-trimpath`：去掉构建路径，让不同机器编出的二进制可复现
- `-ldflags "-X main.version=..."`：把版本号打进二进制，`--version` 直接打印它（Sunshine 的 `version` 命令就是这么注入的）
- `-s -w`：去掉符号表与调试信息，减小体积（需要调试时别加）

**可选 lint**：`go vet` 是底线；团队再接 `golangci-lint`（内含 `staticcheck`、`ineffassign`、`gosec` 等），配 `.golangci.yml` 后 `golangci-lint run ./...` 一次跑全。lint 规则进 CI，不要只在本地跑。

**构建选型速查：**

| 情况 | 做法 |
|---|---|
| 只发库（别人 `import`） | 不用打包，`go build ./...` + `go vet` + 测试即可 |
| 发 CLI 单二进制 | `CGO_ENABLED=0` 静态编译 + GoReleaser（见阶段 7） |
| 要最小体积 | `-ldflags "-s -w"`，或再用 `upx`（启动略慢，酌情） |
| 依赖 cgo（如 SQLite 驱动） | 优先选纯 Go 实现（如 `modernc.org/sqlite`），跨平台编译才无负担 |

### 阶段 4——分发形态（module、版本与 `go install`）

这是 Go SDK 最容易出错的地方：分发靠 module 代理与 tag，不靠注册表上传。

**五条不变规则：**

1. `go.mod` 的 module 路径就是 import 路径，发布后不要改
2. v2 及以上大版本必须在 module 路径后加 `/v2`（`module github.com/acme/sdk/v2`），否则旧版用户无法同时引用两个大版本
3. 版本号就是 git tag：`v1.2.3`，不要在 `go.mod` 里手写版本
4. 库不要在 `go.mod` 里留 `replace`（那是本地开发用的，提交前去掉）
5. `go.sum` 必须提交，`vendor/` 可选（离线构建才需要）

**语义化版本与 tag：**

```
v0.1.0              # 0.x：小版本也可以含破坏性变更
v1.0.0-alpha.0      # 内部试水
v1.0.0-beta.0       # 功能完整，API 还可能动
v1.0.0-rc.0         # 冻结，只修 blocker
v1.0.0              # 稳定版
v1.0.1              # 稳定版补丁
v1.1.0-beta.0       # 下个小版本的 beta，同时 v1.0.x 继续打补丁
```

**消费者怎么拿到你的库：**

```bash
# 别人用你的库：go get 拉取指定版本（走 GOPROXY，默认 proxy.golang.org）
go get github.com/acme/sdk@v1.2.3
go get github.com/acme/sdk/v2@v2.0.0

# 你的 CLI：用户直接装二进制，不用 clone 仓库
go install github.com/acme/mytool/cmd/mytool@v1.2.3

# 固定到某个提交（未打 tag 时）：伪版本由 Go 自动生成
go get github.com/acme/sdk@v0.0.0-20261109120000-abcdef123456
```

**多 module 大仓**：多个可独立发布的 module 放同一仓库时，根目录加 `go.work`，本地开发一次改多个 module 不用 `replace`：

```go
// go.work（只用于本地开发，不随发布走）
go 1.24

use (
    ./sdk
    ./cmd/mytool
)
```

注意：`go.work` 不要提交进只发单个 module 的仓库；CI 里 `go test ./...` 认 `go.work`，发布流程认各个 `go.mod`。

### 阶段 5——多包流水线（CI 与任务编排）

SDK 能独立构建后，接进 CI，让“只改 SDK 时只测 SDK、只发 SDK”成为默认。

**最小可用 GitHub Actions（构建 + 测试 + lint）：**

```yaml
# .github/workflows/ci.yml
name: CI
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        go: ["1.23", "1.24"]
        os: [ubuntu-latest, macos-latest, windows-latest]
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go }}
          cache: true
      - run: gofmt -l .
      - run: go vet ./...
      - run: go test -race ./...
```

**关键规则：**

- 每个可发布单元（module / 二进制）有自己的构建与测试命令，根目录只做编排（`make` / `just` / CI job 组合），不要把构建逻辑全塞根 `Makefile`
- CI 缓存 Go module 与构建缓存（`actions/setup-go` 的 `cache: true`），不要每次全量下载
- 跨平台矩阵至少覆盖 `linux` / `darwin` / `windows` × `amd64` / `arm64`（发二进制时）；纯库可只测 `linux` + 一个 Go 版本，省 CI 时间
- `go test -race` 只在有并发的包上跑全量也可以，但不要完全不跑——数据竞争在单测里不报错，上线才炸

**本地常用命令：**

```bash
go test ./...                          # 全仓测试
go test -run TestClient -v ./pkg/sdk/  # 只跑某个包/某个测试
go test -cover ./...                   # 覆盖率
go test ./... -count=1                 # 禁用缓存重跑（定位偶发失败）
```

### 阶段 6——验证

发布前三道检查缺一不可：静态检查、测试、冒烟安装。

```bash
# 1. 静态检查与格式
gofmt -l . && go vet ./...

# 2. 全量测试（含竞态）
go test -race ./...

# 3. 冒烟：换一个干净目录，像陌生人一样引用你的库
mkdir /tmp/smoke && cd /tmp/smoke
go mod init smoke
go get github.com/acme/sdk@v1.2.3
cat > main.go <<'EOF'
package main

import (
    "fmt"
    "github.com/acme/sdk/pkg/sdk"
)

func main() {
    c, err := sdk.NewClient("https://example.com")
    if err != nil {
        panic(err)
    }
    fmt.Println(c)
}
EOF
go run .
```

**为什么要冒烟？** `go vet` 与单测都在你自己的仓库里跑，测不到“别人 import 时缺依赖、路径写错、tag 打错”的问题。冒烟用真实的 module 代理路径走一遍，是唯一能提前发现分发出错的办法。发 CLI 时同理：在干净目录 `go install pkg@version` 装一次，再跑 `--help` 与 `--version`。

**发布前门禁（进 CI 或发版脚本，不要只靠记忆）：**

```bash
gofmt -l .            # 必须零输出
go vet ./...          # 必须通过
go test -race ./...   # 必须通过
go mod tidy && git diff --exit-code go.mod go.sum   # tidy 后无改动
```

### 阶段 7——发布生命周期（semver tag + GoReleaser + GitHub Release）

Go 库的发布就是打 tag；Go CLI 的发布是打 tag 后由 GoReleaser 编出多平台二进制、发 GitHub Release。

**版本线：**

```
0.x.y           # 0.x：小版本可含破坏性变更
1.0.0-alpha.0   # 内部试水
1.0.0-beta.0    # 功能完整，API 还可能动
1.0.0-rc.0      # 冻结，只修 blocker
1.0.0           # 稳定版
1.0.1           # 稳定版补丁
1.1.0-beta.0    # 下个小版本的 beta，同时 v1.0.x 继续打补丁
```

**预发布绝不污染稳定版：** 用户 `go get pkg@latest` / `go install pkg@latest` 拿到的永远是最高稳定版 tag；beta/rc 只给显式写版本号的人。Go module 没有 npm 的 dist-tag 概念——预发布 tag 本身就是隔离机制：`v1.0.0-beta.0` 永远不会被当成 `latest`。

**GoReleaser 最小可用配置（CLI 用）：**

```yaml
# .goreleaser.yaml
version: 2
builds:
  - main: ./cmd/mytool
    binary: mytool
    env: [CGO_ENABLED=0]
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
    ldflags:
      - -s -w -X main.version={{ .Version }} -trimpath
archives:
  - format: tar.gz
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
checksum:
  name_template: checksums.txt
release:
  github:
    owner: <组织>
    name: <仓库>
```

**发布流程（GitHub Actions）：**

```yaml
# .github/workflows/release.yml
name: Release
on:
  push:
    tags: ["v*"]
permissions:
  contents: write
jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version: "1.24"
          cache: true
      - uses: goreleaser/goreleaser-action@v6
        with:
          version: latest
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

**发布命令序列：**

```bash
# 1. 功能完成，切 beta 线
git tag v1.0.0-beta.0 && git push origin v1.0.0-beta.0
# → GoReleaser 自动编包、发 GitHub Release（预发布）

# 2. 冻结，切 rc
git tag v1.0.0-rc.0 && git push origin v1.0.0-rc.0

# 3. 稳定，发 1.0.0
git tag v1.0.0 && git push origin v1.0.0
# → 用户 go install pkg@v1.0.0 / go get pkg@v1.0.0 拿到稳定版

# 4. 开下一个小版本的 beta 线
git tag v1.1.0-beta.0 && git push origin v1.1.0-beta.0
```

**发现问题后撤回：** Go module 没有“下架”按钮。用 `retract` 声明某版本有问题，`go get` 会跳过它并警告：

```go
// go.mod
retract v1.0.0 // 含数据竞争问题，请用 v1.0.1
retract [v1.0.0-beta.0, v1.0.0-rc.0] // 区间撤回
```

`retract` 只影响新安装，已装的用户不受影响；同时发修复版 tag，给用户一条明确的升级路。不要删 tag 重打——module 代理已缓存旧 tag，删了重打会让不同用户拿到不同内容。

**CHANGELOG：** 每个 tag 配一份变更记录（手写或用 `git log v1.0.0..v1.1.0 --oneline` 生成骨架再润色）；破坏性变更放最前面写清迁移办法。

---

## 速查

### 文件 / 字段速查表

| 文件 | 管什么 | 快速检查 |
|---|---|---|
| `go.mod` | module 路径、Go 版本、依赖 | `go mod tidy` 后无多余改动；v2+ 带 `/v2` 后缀 |
| `go.sum` | 依赖哈希 | 已提交；不要手改 |
| `go.work` | 本地多 module 开发 | 只放本地，不要随单 module 发布 |
| `cmd/<名>/main.go` | CLI 入口 | 只做接线，逻辑在 `pkg/` / `internal/` |
| `pkg/<库>/` | 公开库 | 只导出契约；内部实现放 `internal/` |
| `internal/` | 内部实现 | 编译器拦外部 import |
| `.goreleaser.yaml` | 多平台构建与发布 | `goreleaser check` 通过 |
| `.github/workflows/release.yml` | tag 触发发布 | 只在 `v*` tag 上跑；`contents: write` |

### 发布 tag 速查

| Tag | 含义 | 用户怎么拿到 |
|---|---|---|
| `v1.2.3` | 当前稳定版 | `go get pkg@v1.2.3` / `go install pkg@v1.2.3` |
| `v1.3.0-beta.0` | 下个小版本的 beta | 显式写版本号才拿到 |
| `v1.3.0-rc.0` | 冻结，只修 blocker | 显式写版本号才拿到 |
| `v2.0.0` | 大版本（module 路径带 `/v2`） | `go get pkg/v2@v2.0.0` |
| 伪版本 | 未打 tag 的某次提交 | `go get pkg@<提交>` 自动生成 |

### 常用一行命令

```bash
# 新建 module
go mod init github.com/acme/sdk

# 同步依赖
go mod tidy

# 只测改了的包（配合 git）
go test $(go list ./... | grep -v /internal/irrelevant)

# 装自己的 CLI 看效果
go install ./cmd/mytool && mytool --version

# 发版前本地演练（不真发）
goreleaser release --snapshot --clean

# 看某版本被谁依赖、依赖图
go list -m all
go mod graph | grep acme/sdk
```

---

## 发布前检查清单

每发一个版本过一遍。跳过任何一项，都是在给未来的自己挖坑。

### 构建产物

- [ ] `go build ./...` 通过；CLI 的多平台交叉编译通过（`GOOS`/`GOARCH` 矩阵）
- [ ] `gofmt -l .` 零输出；`go vet ./...` 通过
- [ ] `go test -race ./...` 通过
- [ ] 二进制体积合理（`ls -lh mytool` 看一眼，别带进调试符号或测试数据）
- [ ] `--version` 打印的版本号与 tag 一致（ldflags 注入无误）

### go.mod

- [ ] module 路径正确；v2+ 带 `/v2` 后缀
- [ ] `go mod tidy` 后 `go.mod` / `go.sum` 无意外改动
- [ ] 没有残留的 `replace`（本地开发用完已去掉）
- [ ] `retract` 只写真正有问题的版本，不写“预防性”撤回

### 验证

- [ ] 冒烟：干净目录 `go get` / `go install` 指定版本能跑通
- [ ] 公开 API 变更已检查：导出新增/删除/改签名，破坏性变更已在 CHANGELOG 置顶说明
- [ ] `go doc ./pkg/sdk` 输出的公开面与预期一致，没有内部类型泄漏

### 发布

- [ ] tag 名正确（`v1.0.0-beta.0` 不是 `1.0.0-beta`，不是 `v1.0.0_beta`）
- [ ] 预发布 tag 不会被当成稳定版（Go module 天然隔离，仍要确认 Release 标记为预发布）
- [ ] CHANGELOG 已写，破坏性变更有迁移办法
- [ ] GitHub Release 里有各平台产物 + `checksums.txt`
- [ ] 发现问题时：发修复版 + 写 `retract`，不删 tag 重打

---

## 常见错误

| 错误 | 会出什么问题 | 修复 |
|---|---|---|
| 公开签名里出现 `internal/` 包的类型 | 外部用户 import 失败，库根本没法用 | 公开 API 只用公开包的类型；内部类型转一层再返回 |
| v2 大版本忘了加 `/v2` 后缀 | 用户升级后 import 路径冲突，两个大版本无法共存 | module 路径改为 `.../sdk/v2`，import 路径同步改 |
| 导出接口方法太多 | 以后加一个方法就是破坏性变更，所有实现方都得改 | 接口保持最小；用结构 + 函数式选项代替大接口 |
| 错误只返回 `fmt.Errorf("失败")` 不包 `%w` | 调用方无法用 `errors.Is` / `errors.As` 判断错误类型 | 哨兵错误 + `%w` 包装，保留错误链 |
| `replace` 提交进了 `go.mod` | 别人拉你的库时 replace 指向不存在的本地路径，直接失败 | 发布前去掉 `replace`；本地开发用 `go.work` |
| 把 tag 删了重打 | module 代理缓存了旧 tag，不同用户拿到不同内容 | 永不删 tag；发新版本 + `retract` 旧版本 |
| 预发布当稳定版发 | 用户 `go get @latest` 拿到未冻结的 API | beta/rc 只打预发布 tag，等冻结后再打稳定版 tag |
| `go.mod` 里 Go 版本写得太新 | 低版本 Go 的用户装不上 | `go` 指令写支持的最低版本，CI 矩阵里测它 |
| 零值不可用还不写文档 | 用户 `var c Client` 一调方法就 panic | 要么让零值合法，要么文档置顶写明必须用 `NewClient` |
| 只在 `linux/amd64` 测，发布全平台 | darwin/windows 的路径、信号、换行问题上线才暴露 | 发二进制前 CI 矩阵覆盖目标平台 |
| 把构建逻辑全塞根 `Makefile` | 多 module 时互相牵连，改一个包全量重编 | 每个 module 有自己的构建命令，根目录只编排 |
| 公开 API 用 `any` / `interface{}` 兜底 | 调用方失去类型检查，拼错字段编译器不拦 | 用具体类型 + 泛型；`any` 只留在真正需要动态的地方 |

---

## 与 Sunshine 的对照

Sunshine 本身就是这套做法的实例：单 Go module（`github.com/BondLam/Sunshine`）、`cmd/sunshine` 入口、`internal/` 放全部实现、`go:embed` 内嵌模板随二进制发布、`-ldflags -X` 注入版本、`goreleaser` 打 5 个平台产物、GitHub Release 发版、`sunshine upgrade` 自更新。照着它的仓库结构抄布局，再按本 skill 检查公开面与发布流程即可。
