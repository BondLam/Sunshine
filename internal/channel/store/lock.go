package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// 默认重试间隔与最长等待时间（与上游一致）。
const (
	// DefaultLockRetryIntervalMs 默认重试间隔 25ms。
	DefaultLockRetryIntervalMs = 25
	// DefaultLockMaxWaitMs 默认最长等待 5000ms。
	DefaultLockMaxWaitMs = 5000
)

// AcquireOptions 是 AcquireLock 的选项；0 值即默认值。
type AcquireOptions struct {
	// RetryIntervalMs 重试间隔（毫秒），默认 25。
	RetryIntervalMs int
	// MaxWaitMs 最长等待（毫秒），默认 5000。
	MaxWaitMs int
}

func (o AcquireOptions) interval() time.Duration {
	if o.RetryIntervalMs <= 0 {
		return DefaultLockRetryIntervalMs * time.Millisecond
	}
	return time.Duration(o.RetryIntervalMs) * time.Millisecond
}

func (o AcquireOptions) maxWait() time.Duration {
	if o.MaxWaitMs <= 0 {
		return DefaultLockMaxWaitMs * time.Millisecond
	}
	return time.Duration(o.MaxWaitMs) * time.Millisecond
}

// AcquireLock 用 O_CREATE|O_EXCL 原子创建锁文件，文件内写持有者 pid。
// 锁文件存在时按间隔重试；持有者 pid 已死则窃取锁。超时报错。
func AcquireLock(lockFile string, opts AcquireOptions) error {
	if err := os.MkdirAll(filepath.Dir(lockFile), 0o755); err != nil {
		return fmt.Errorf("创建锁目录失败：%w", err)
	}
	deadline := time.Now().Add(opts.maxWait())
	for {
		f, err := os.OpenFile(lockFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, werr := f.WriteString(strconv.Itoa(os.Getpid()))
			cerr := f.Close()
			if werr != nil {
				return fmt.Errorf("写锁文件失败：%w", werr)
			}
			if cerr != nil {
				return fmt.Errorf("关闭锁文件失败：%w", cerr)
			}
			return nil
		}
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("创建锁文件失败：%w", err)
		}
		if checkAndStealStale(lockFile) {
			continue
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("在 %dms 内未能获得锁 %s", opts.maxWait().Milliseconds(), lockFile)
		}
		time.Sleep(opts.interval())
	}
}

// ReleaseLock 释放锁：仅当锁文件内容等于自身 pid 时才删除，
// 避免误删他人持有的锁。锁文件不存在或不属于自己时静默返回。
func ReleaseLock(lockFile string) {
	data, err := os.ReadFile(lockFile)
	if err != nil {
		return
	}
	if strings.TrimSpace(string(data)) == strconv.Itoa(os.Getpid()) {
		os.Remove(lockFile)
	}
}

// WithLock 在锁内执行 fn，结束后释放锁（即使 fn 报错也释放）。
func WithLock[T any](lockFile string, fn func() (T, error), opts AcquireOptions) (T, error) {
	var zero T
	if err := AcquireLock(lockFile, opts); err != nil {
		return zero, err
	}
	defer ReleaseLock(lockFile)
	return fn()
}

// checkAndStealStale 检查锁文件持有者 pid 是否已死；已死则删除锁文件
// 并返回 true（调用方立即重试创建）。pid 无法解析（文件刚创建、
// 持有者还没来得及写 pid）时，只在文件已存在超过 1 秒才视为陈旧锁，
// 避免并发创建瞬间把别人的新锁误判为死锁窃取。
func checkAndStealStale(lockFile string) bool {
	data, err := os.ReadFile(lockFile)
	if err != nil {
		return false
	}
	holderPid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || holderPid <= 0 {
		if stat, serr := os.Stat(lockFile); serr == nil && time.Since(stat.ModTime()) < time.Second {
			return false
		}
		holderPid = 0
	}
	if holderPid == 0 || !pidAlive(holderPid) {
		if err := os.Remove(lockFile); err != nil {
			return false
		}
		fmt.Fprintf(os.Stderr, "[channel 锁] 已窃取来自已死进程 %d 的陈旧锁 %s\n", holderPid, lockFile)
		return true
	}
	return false
}
