# 迁移清单（manifests/）

本目录存放 Sunshine 自己的版本迁移清单，每份清单对应一个版本，
文件名 `<版本号>.json`（例如 `0.2.0.json`）。

## 当前状态：空

当前版本尚无历史迁移，清单集初始为空。后续每次模板结构变更
（文件改名、目录改名、删除过期文件、新引入配置段落），由该版本
补一份清单；不要为没有结构变更的版本建清单。

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
