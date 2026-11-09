# Sunshine

把 Trellis 的全部功能用 Go 重新实现，CLI 界面与生成的规范文件全部中文。

## 安装

macOS / Linux 一行装好（自动识别系统与架构，校验 sha256）：

```sh
curl -fsSL https://raw.githubusercontent.com/linrong1982/Sunshine/main/install.sh | sh
```

装指定版本：

```sh
curl -fsSL https://raw.githubusercontent.com/linrong1982/Sunshine/main/install.sh | sh -s -- v0.2.0
```

Windows 请到 [Releases](https://github.com/linrong1982/Sunshine/releases) 页下载对应 zip，解压后把 `sunshine.exe` 放进 PATH。

已有 Go 的机器也可以：

```sh
go install github.com/linrong1982/Sunshine/cmd/sunshine@latest
```

装好后升级只需：

```sh
sunshine upgrade
```

## 快速上手

```sh
cd 你的项目
sunshine init        # 生成中文 .sunshine/ 骨架（规范、任务、工作流）
sunshine --help      # 看全部命令
```

## 更多

- 完整规划与各阶段实施记录：[PLAN.md](PLAN.md)
