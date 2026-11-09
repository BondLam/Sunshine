#!/bin/sh
# Sunshine 一键安装脚本
#
# 用法（装最新版）：
#   curl -fsSL https://raw.githubusercontent.com/linrong1982/Sunshine/main/install.sh | sh
#
# 装指定版本（二选一）：
#   curl -fsSL .../install.sh | sh -s -- v0.2.0
#   SUNSHINE_VERSION=v0.2.0 sh install.sh
#
# 装到指定目录（默认 /usr/local/bin，不可写时退到 ~/.local/bin）：
#   SUNSHINE_INSTALL_DIR="$HOME/bin" sh install.sh
#
# 只支持 macOS / Linux（x86_64 与 arm64）。Windows 请到 Release 页下载 zip：
#   https://github.com/linrong1982/Sunshine/releases
set -eu

REPO="linrong1982/Sunshine"
PROG="sunshine"

die() { printf '%s\n' "install.sh: $*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "需要 $1，请先安装再重试"; }

need curl
need tar

# ---- 版本：参数 > 环境变量 > 最新 ----
VERSION="${1:-${SUNSHINE_VERSION:-latest}}"
case "$VERSION" in
  -h|--help)
    cat <<'EOF'
用法：sh install.sh [版本]
  curl -fsSL https://raw.githubusercontent.com/linrong1982/Sunshine/main/install.sh | sh
  curl -fsSL .../install.sh | sh -s -- v0.2.0
  SUNSHINE_VERSION=v0.2.0 sh install.sh
  SUNSHINE_INSTALL_DIR="$HOME/bin" sh install.sh
只支持 macOS / Linux（amd64 与 arm64）；Windows 请到 Release 页下载 zip。
EOF
    exit 0 ;;
esac
if [ "$VERSION" = "latest" ]; then
  echo "正在查询最新版本……"
  VERSION="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | grep '"tag_name":' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"
  [ -n "$VERSION" ] || die "查不到最新版本，请检查网络，或用 SUNSHINE_VERSION=v0.2.0 指定版本"
fi
# tag 带 v 前缀（如 v0.2.0），包名里的版本号不带 v（如 sunshine_0.2.0_…）。
TAG="$VERSION"
case "$TAG" in v*) ;; *) TAG="v$TAG" ;; esac
VER="${TAG#v}"

# ---- 平台与架构 ----
OS="$(uname -s)"
case "$OS" in
  Darwin) OS="darwin" ;;
  Linux)  OS="linux" ;;
  *) die "不支持的操作系统 $OS（只支持 macOS / Linux）；Windows 请到 Release 页下载 zip" ;;
esac
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) die "不支持的架构 $ARCH（只支持 amd64 / arm64）" ;;
esac

ARCHIVE="${PROG}_${VER}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/$REPO/releases/download/$TAG/$ARCHIVE"
echo "安装 $PROG $TAG（$OS/$ARCH）……"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT INT TERM
cd "$TMP"

# ---- 下载 + 校验 ----
curl -fsSL -o "$ARCHIVE" "$URL" || die "下载失败：$URL（请确认该版本已发布）"
curl -fsSL -o checksums.txt "https://github.com/$REPO/releases/download/$TAG/checksums.txt" \
  || die "下载校验文件失败"
if command -v sha256sum >/dev/null 2>&1; then
  grep " $ARCHIVE\$" checksums.txt | sha256sum -c - >/dev/null \
    || die "校验失败：下载的文件与 checksums.txt 不一致"
elif command -v shasum >/dev/null 2>&1; then
  grep " $ARCHIVE\$" checksums.txt | shasum -a 256 -c - >/dev/null \
    || die "校验失败：下载的文件与 checksums.txt 不一致"
else
  echo "提示：本机没有 sha256sum/shasum，跳过校验"
fi

tar -xzf "$ARCHIVE"
[ -x "$PROG" ] || die "解压后找不到可执行的 $PROG"

# ---- 落盘：默认 /usr/local/bin，不可写时退到 ~/.local/bin；
#      SUNSHINE_INSTALL_DIR 显式指定时，建不出或不可写就直接报错 ----
if [ -n "${SUNSHINE_INSTALL_DIR:-}" ]; then
  DIR="$SUNSHINE_INSTALL_DIR"
  mkdir -p "$DIR" || die "无法创建安装目录 $DIR"
  [ -w "$DIR" ] || die "安装目录 $DIR 不可写"
else
  DIR="/usr/local/bin"
  if [ ! -w "$DIR" ]; then
    DIR="$HOME/.local/bin"
    mkdir -p "$DIR" || die "无法创建 $DIR"
    echo "提示：/usr/local/bin 不可写，改装到 $DIR"
  fi
fi
install -m 755 "$PROG" "$DIR/$PROG"

# ---- 验证 ----
if command -v "$PROG" >/dev/null 2>&1; then
  "$PROG" --version
else
  "$DIR/$PROG" --version
  case ":$PATH:" in
    *":$DIR:"*) ;;
    *) echo "提示：$DIR 不在 PATH 里，请加一行：export PATH=\"\$PATH:$DIR\"" ;;
  esac
fi
echo "安装完成：$DIR/$PROG"
