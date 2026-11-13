package supervisor

import (
	"fmt"
	"os"

	"github.com/linrong007/Sunshine/internal/channel/store"
)

// WriteSessionID 持久化 worker 会话 id（<worker>.session-id），
// 把 worker 的会话身份桥接到 channel，供将来 --resume 复用。
// claude 从 system init 行拿 session_id；codex 从 thread/start 结果
// 拿 thread id（与 session id 同值）。
func WriteSessionID(ref store.ChannelRef, worker, id string) error {
	p, err := store.WorkerFile(ref.Name, worker, "session-id", ref.Project)
	if err != nil {
		return err
	}
	if err := os.WriteFile(p, []byte(id), 0o644); err != nil {
		return fmt.Errorf("写 worker 会话 id 失败：%w", err)
	}
	return nil
}

// WriteThreadID 持久化 codex 线程 id（<worker>.thread-id）。
func WriteThreadID(ref store.ChannelRef, worker, id string) error {
	p, err := store.WorkerFile(ref.Name, worker, "thread-id", ref.Project)
	if err != nil {
		return err
	}
	if err := os.WriteFile(p, []byte(id), 0o644); err != nil {
		return fmt.Errorf("写 worker 线程 id 失败：%w", err)
	}
	return nil
}

// ReadSessionID 读 worker 的会话 id（--resume 用）；不存在时返回 ""。
func ReadSessionID(ref store.ChannelRef, worker string) (string, error) {
	p, err := store.WorkerFile(ref.Name, worker, "session-id", ref.Project)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("读 worker 会话 id 失败：%w", err)
	}
	return string(data), nil
}

// ReadThreadID 读 codex worker 的线程 id；不存在时返回 ""。
func ReadThreadID(ref store.ChannelRef, worker string) (string, error) {
	p, err := store.WorkerFile(ref.Name, worker, "thread-id", ref.Project)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("读 worker 线程 id 失败：%w", err)
	}
	return string(data), nil
}
