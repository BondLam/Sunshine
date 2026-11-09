# Sunshine 模板市场（Marketplace）说明

本文档描述 `sunshine init --registry` 接受的模板仓库格式。
结构以实查为准（2026-11-09 核查官方模板仓库
`mindfold-ai/marketplace` 的 GitHub API 列表），不照抄上游
`marketplace/README.md`——该 README 称含 `commands/` 与 `agents/`
目录，实查并无此二目录。

## 仓库结构

```
<仓库根>/
├── index.json        # 模板索引（version 1），有它就是模板市场模式
├── specs/            # spec 模板，每子目录一个模板
├── skills/           # skill 模板，每子目录一个 skill
├── workflows/        # workflow 模板，每子目录一个工作流
└── README.md         # 可选
```

没有 `commands/`、`agents/` 目录。`index.json` 在仓库根
（不是 `marketplace/index.json`——那是 Trellis 单体仓库里
`marketplace/` 子目录的写法；用 `--registry` 指过去时，
来源要写到 `marketplace` 子目录，如 `gh:mindfold-ai/Trellis/marketplace`）。

## index.json

```json
{
  "version": 1,
  "templates": [
    {"id": "electron-fullstack", "type": "spec", "name": "...", "path": "specs/electron-fullstack"},
    {"id": "sunshine-meta", "type": "skill", "name": "...", "path": "skills/sunshine-meta"},
    {"id": "native", "type": "workflow", "name": "...", "path": "workflows/native/workflow.md"}
  ]
}
```

- `id`、`type`、`name`、`path` 为必填；`description`、`tags` 可选。
- `path` 相对 registry 根（index.json 所在目录）。
- 已确认的 10 个条目：3 spec（`electron-fullstack`、`nextjs-fullstack`、
  `cf-workers-fullstack`）、4 skill（`sunshine-meta`、
  `sunshine-spec-bootstrap`、`frontend-fullchain-optimization`、
  `mem-recall`）、
  3 workflow（`native`、`tdd`、`channel-driven-subagent-dispatch`）。
  上游 `trellis-spec-bootstarp` 是错拼，Sunshine 用
  `sunshine-spec-bootstrap`，不沿用。`mem-recall` 已收录（`sunshine mem`
  的用户入口）。

## 安装位置

| type    | 安装目录                    |
|---------|-----------------------------|
| spec    | `.sunshine/spec`            |
| skill   | `.agents/skills/<id>`       |
| command | `.claude/commands/<id>`     |
| full    | 项目根                      |
| workflow| 不走 `--registry`，请用 `sunshine workflow --template <id>` |

## 两种模式

- **模板市场模式**（有 `index.json`）：须用 `--template <id>` 指定条目。
- **直接下载模式**（无 `index.json`）：整个 registry 内容装进
  `.sunshine/spec`；若 registry 根下存在 `marketplace/specs/`，
  以该目录为模板；否则以 registry 根本身为模板（私有 spec 仓库）。

## 来源写法

- `gh:owner/repo`，可带分支/子目录：`gh:owner/repo/marketplace#dev`
- 完整 git URL、`https://github.com/owner/repo`（含 `/tree/<分支>/<路径>`）
- SSH：`git@github.com:owner/repo`、`ssh://git@host:2222/owner/repo`
- 本地 git 仓库路径（Sunshine 补充）：`/path/to/repo`、`./repo`、`file:///path/repo`
  （可带子目录，如 `/path/to/repo/marketplace`）

## 冲突策略

默认 `skip`（目标已存在则跳过）；`--overwrite` 覆盖、
`--append` 只补缺失文件，两者仅配合 `--template` 使用。

## 持续刷新

装完后来源与模板记进 `.sunshine/config.yaml` 的 `registry.spec`，
`sunshine update` 按它重新拉取 spec 模板，本地改动走正常冲突流程。
