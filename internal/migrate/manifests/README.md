# 迁移清单（manifests/）

本目录存放 Sunshine 自己的版本迁移清单，每份清单对应一个版本，
文件名 `<版本号>.json`（例如 `0.2.0.json`）。

## 当前状态：已有第一份真实清单

- `0.2.0.json`（2026-11-09 落地）：Skill-first 架构迁移——6 条改名
  （Claude / Cursor 命令改 skill、`.agents/skills/` 未加前缀的 skill
  加 `sunshine-` 前缀）与 4 条 safe-file-delete（退役的 onboard /
  parallel 旧命令与旧 skill）。蓝本是 Trellis 0.5.0-beta.0 的
  Skill-first 迁移（该版含 68 条改名与 138 条 safe-file-delete）；
  只取“改了什么”的事实，未复制上游代码与原文；路径按 Sunshine 的
  `.claude` / `.cursor` / `.agents` 布局与 `sunshine-` 前缀改写。
  0.1.0 模板里本就用统一 skill 名的项目不受影响（源文件不存在的项
  自动跳过）。回归测试见 `internal/update/migrate020_test.go`
  （门禁 / 试运行 / `--migrate --force` / 备份后执行四路）。

## 新增清单（之后每加一份照此办理）

1. **取材**：只写 Sunshine 自己版本线上的真实结构变更（文件改名、
   目录改名、删除过期文件、新引入配置段落）；不要为没有结构变更的
   版本建清单；不要照搬其他项目的清单（路径与结构都不同）。
   如需参考上游历史，只取“改了什么”的事实，不抄代码、不直接发布
   英文原文，蓝本出处写进清单的 `notes`。
2. **文件**：`manifests/<目标版本号>.json`（例如 `0.3.0.json`），
   版本号须与该版 CLI 版本一致；清单只在项目版→CLI 版区间
   `(旧版, 新版]` 内生效。
3. **内容**：`version` / `description` / `migrations`（`rename` /
   `rename-dir` / `delete` / `safe-file-delete`，`from` / `to` 均为
   相对项目根的路径）必填其余按需；破坏性变更置 `breaking` 与
   `recommendMigrate` 为 true（无 `--migrate` 时 update 硬停）。
   `safe-file-delete` 的 `allowed_hashes` 填旧版模板文件内容的
   原始字节 SHA-256（与 `.template-hashes.json` 的算法一致，
   不做换行符归一化），可用 `sha256sum <文件>` 计算；用户改过
   的文件哈希不命中，一定保留。
4. **禁区**：受保护的用户数据路径（`.sunshine/workspace`、
   `.sunshine/tasks`、`.sunshine/spec`、`.sunshine/.developer`、
   `.sunshine/.current-task`）永不迁移、不删除，不要写进清单。
   改名源不得是当前模板集合里仍有的路径（否则 update 会在迁移后
   又把它当新文件写回来）。
5. **验证**：在 `internal/migrate/migrate_test.go` 的内嵌清单测试
   里更新版本断言；在 `internal/update/` 补一轮 CLI 级回归
   （门禁 / 试运行 / `--migrate` 执行 / `.version` 更新 / 备份含
   更新前内容）；再用真实二进制搭夹具项目跑交互与非交互两路
   （夹具做法：真实 `init` 建 0.1.0 项目，植入旧版文件并记哈希，
   删掉改名目标的文件与哈希条目，再以新版二进制跑 `update`）。
6. **发布**：只加文件，不改代码——清单随二进制内嵌发布
   （`//go:embed manifests`），`update` 在 `MigrationRegistry`
   为 nil 时自动回落到内嵌清单集。

## 清单格式（JSON）

```json
{
  "version": "0.2.0",
  "description": "命令目录改名：旧命令统一搬到新目录",
  "breaking": true,
  "recommendMigrate": true,
  "changelog": "旧命令目录 .sunshine/commands-old/ 改名为 .sunshine/commands/。",
  "migrations": [
    {
      "type": "rename-dir",
      "from": ".sunshine/commands-old",
      "to": ".sunshine/commands",
      "description": "命令目录改名"
    },
    {
      "type": "rename",
      "from": ".sunshine/skills/old-skill.md",
      "to": ".sunshine/skills/new-skill.md",
      "description": "技能文件改名",
      "reason": "旧文件在 0.1.0 里没有进哈希清单，改动检测可能误判，确认一下更稳妥。"
    },
    {
      "type": "safe-file-delete",
      "from": ".sunshine/commands/deprecated.md",
      "description": "删除已废弃的旧命令",
      "allowed_hashes": ["<该文件模板内容的 SHA-256>"]
    }
  ],
  "configSectionsAdded": [
    {
      "file": ".sunshine/config.yaml",
      "sentinel": "codex:",
      "sectionHeading": "Codex 配置"
    }
  ],
  "notes": "其他需要说明的事项。"
}
```

字段说明：

- `version`（必填）：清单对应的目标版本。
- `description`：本版改动简述；`changelog`：展示给用户的详细记录。
- `breaking`：含破坏性变更；`recommendMigrate`：建议用户用 `--migrate`。
- `migrations`：迁移动作，`type` 取值：
  - `rename` / `rename-dir`：改名（需 `to`）；只有 `--migrate` 才执行。
  - `delete`：删除文件；只有 `--migrate` 才执行。
  - `safe-file-delete`：删除过期文件；文件内容哈希命中 `allowed_hashes`
    时无需 `--migrate` 也自动执行，用户改过的文件一定保留。
  - `from` / `to` 均为相对项目根的路径；`reason` 是向用户确认时展示的
    版本相关背景（1–2 句）。
- `configSectionsAdded`：本版新引入的配置段落。`sentinel` 是判定段落
  是否已存在的子串（取稳定的新顶层键）；`sectionHeading` 是新版模板
  分隔块里的 `# <标题>` 文本；update 按 sentinel 判定后追加，
  不覆盖用户的其他定制。
- `migrationGuide` / `aiInstructions`（可选）：详细的迁移指南与给 AI
  助手的说明。

## 注意事项

- 清单路径一律用 Sunshine 自己的 `.sunshine/` 布局，不要照搬其他
  项目的清单（路径与结构都不同）。
- 受保护的用户数据路径（`.sunshine/workspace`、`.sunshine/tasks`、
  `.sunshine/spec`、`.sunshine/.developer`、`.sunshine/.current-task`）
  永不迁移、不删除，不要写进清单。
- `safe-file-delete` 的 `allowed_hashes` 填模板文件内容的原始字节
  SHA-256（与 `.template-hashes.json` 的算法一致，不做换行符归一化），
  可用 `sha256sum <文件>` 计算。
- 清单文件损坏或缺少 `version` 时，加载会跳过该文件并警告，不中断更新。
