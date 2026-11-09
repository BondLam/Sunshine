package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// 尾部扫描用的窗口大小。
const (
	readTailBytes = 4096
	tailScanChunk = 64 * 1024
)

// parseSidecar 解析 sidecar 内容；缺失或非纯数字返回 -1。
func parseSidecar(text string) int64 {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return -1
	}
	if !regexp.MustCompile(`^[0-9]+$`).MatchString(trimmed) {
		return -1
	}
	n, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || n < 0 {
		return -1
	}
	return n
}

func readSidecar(sidecarPath string) int64 {
	data, err := os.ReadFile(sidecarPath)
	if err != nil {
		return -1
	}
	return parseSidecar(string(data))
}

// WriteSidecar 原子写 sidecar：先写临时文件再重命名。
func WriteSidecar(sidecarPath string, seq int64) error {
	if err := os.MkdirAll(filepath.Dir(sidecarPath), 0o755); err != nil {
		return fmt.Errorf("创建 sidecar 目录失败：%w", err)
	}
	tmp := fmt.Sprintf("%s.tmp.%d.%d", sidecarPath, os.Getpid(), time.Now().UnixNano())
	if err := os.WriteFile(tmp, []byte(fmt.Sprintf("%d\n", seq)), 0o644); err != nil {
		return fmt.Errorf("写 sidecar 临时文件失败：%w", err)
	}
	if err := os.Rename(tmp, sidecarPath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("重命名 sidecar 失败：%w", err)
	}
	return nil
}

// readLastJSONLSeq 从 JSONL 尾部读最后一条事件的 seq：先读尾部
// 4096 字节，拿不到时全量扫描。文件不存在或为空返回 0；
// 文件有内容但无一条可恢复的 seq 时报错（不能猜，否则会重发 seq）。
func readLastJSONLSeq(jsonlPath string) (int64, error) {
	stat, err := os.Stat(jsonlPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if stat.Size() == 0 {
		return 0, nil
	}
	seqFromBuffer := func(buf []byte) (int64, bool) {
		lines := bytes.Split(buf, []byte("\n"))
		for i := len(lines) - 1; i >= 0; i-- {
			line := bytes.TrimSpace(lines[i])
			if len(line) == 0 {
				continue
			}
			var parsed struct {
				Seq *int64 `json:"seq"`
			}
			if err := json.Unmarshal(line, &parsed); err != nil {
				continue
			}
			if parsed.Seq != nil {
				return *parsed.Seq, true
			}
		}
		return 0, false
	}

	tailLen := stat.Size()
	if tailLen > readTailBytes {
		tailLen = readTailBytes
	}
	f, err := os.Open(jsonlPath)
	if err != nil {
		return 0, err
	}
	buf := make([]byte, tailLen)
	if _, err := f.ReadAt(buf, stat.Size()-tailLen); err != nil {
		f.Close()
		return 0, err
	}
	f.Close()
	usable := buf
	if stat.Size() > tailLen {
		if idx := bytes.IndexByte(buf, '\n'); idx >= 0 {
			usable = buf[idx+1:]
		} else {
			usable = nil
		}
	}
	if len(usable) > 0 {
		if seq, ok := seqFromBuffer(usable); ok {
			return seq, nil
		}
	}

	data, err := os.ReadFile(jsonlPath)
	if err != nil {
		return 0, err
	}
	if seq, ok := seqFromBuffer(data); ok {
		return seq, nil
	}
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(bytes.TrimSpace(line)) > 0 {
			return 0, fmt.Errorf("无法从 %s 恢复 channel seq", jsonlPath)
		}
	}
	return 0, nil
}

// TruncateIncompleteTail 截断 JSONL 的撕裂尾部：文件末尾没有换行
// 说明最后一行是在追加中途被打断的半行，截到上一个完整行末尾。
// 完整但解析失败的行是文件中部损坏，不是撕裂尾部，不动。
// 调用方必须持有 channel 锁。
func TruncateIncompleteTail(jsonlPath string) error {
	f, err := os.OpenFile(jsonlPath, os.O_RDWR, 0o644)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("打开事件日志失败：%w", err)
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return err
	}
	if stat.Size() == 0 {
		return nil
	}
	last := make([]byte, 1)
	if _, err := f.ReadAt(last, stat.Size()-1); err != nil {
		return err
	}
	if last[0] == '\n' {
		return nil
	}
	offset := stat.Size()
	for offset > 0 {
		start := offset - tailScanChunk
		if start < 0 {
			start = 0
		}
		length := offset - start
		buf := make([]byte, length)
		if _, err := f.ReadAt(buf, start); err != nil {
			return err
		}
		if idx := bytes.LastIndexByte(buf, '\n'); idx >= 0 {
			return f.Truncate(start + int64(idx) + 1)
		}
		offset = start
	}
	return f.Truncate(0)
}

// ReconcileSeqFile 以 JSONL 尾部扫描为准修复 .seq sidecar，返回
// 最后一条事件的 seq。sidecar 缺失、损坏、落后或超前都会被修复。
// 调用方必须持有 channel 锁（当后续还要追加写盘时）。
func ReconcileSeqFile(jsonlPath string, sidecarPath string) (int64, error) {
	sidecar := readSidecar(sidecarPath)
	jsonlTail, err := readLastJSONLSeq(jsonlPath)
	if err != nil {
		return 0, err
	}
	if sidecar != jsonlTail {
		if err := WriteSidecar(sidecarPath, jsonlTail); err != nil {
			return 0, err
		}
	}
	return jsonlTail, nil
}

// ReconcileSeq 以 JSONL 尾部扫描为准修复指定 channel 的 .seq sidecar，
// 返回最后一条事件的 seq。本函数自身不加锁：仅用于读侧对账；
// 写路径（AppendEvent）已在锁内调用 ReconcileSeqFile。
func ReconcileSeq(ref ChannelRef) (int64, error) {
	jsonl, err := EventsPath(ref.Name, ref.Project)
	if err != nil {
		return 0, err
	}
	sidecar, err := SeqSidecarPath(ref.Name, ref.Project)
	if err != nil {
		return 0, err
	}
	return ReconcileSeqFile(jsonl, sidecar)
}
