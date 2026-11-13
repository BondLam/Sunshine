package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/linrong007/Sunshine/internal/channel"
	"github.com/linrong007/Sunshine/internal/channel/store"
	"github.com/spf13/cobra"
)

// testChannelForumRoot 建一个只含 channel 命令（含 forum 子命令）的测试根命令。
func testChannelForumRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "sunshine",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	RegisterChannelCommand(root)
	RegisterChannelForumCommand()
	return root
}

// channelForumFixture 在临时存储根里建 forum/chat 两个频道，工作目录切到临时项目。
type channelForumFixture struct {
	root    string
	proj    string
	project string
}

func setupChannelForumFixture(t *testing.T) channelForumFixture {
	t.Helper()
	root := t.TempDir()
	proj := t.TempDir()
	t.Setenv("SUNSHINE_CHANNEL_ROOT", root)
	t.Chdir(proj)
	fs := channel.NewFileStore()
	project := store.ProjectKey(proj)
	for _, item := range []struct{ name, typ string }{
		{"board", "forum"},
		{"talk", "chat"},
	} {
		if _, err := fs.AppendEvent(item.name, project, map[string]any{
			"kind": "create", "by": "main", "type": item.typ,
		}); err != nil {
			t.Fatalf("建频道 %s 失败：%v", item.name, err)
		}
	}
	return channelForumFixture{root: root, proj: proj, project: project}
}

// runChannelForumArgs 执行一次 channel forum 命令并返回标准输出；出错时返回错误。
func runChannelForumArgs(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := testChannelForumRoot()
	root.SetArgs(args)
	return captureStdout(t, func() error {
		return root.Execute()
	})
}

func TestChannelForumFullFlow(t *testing.T) {
	setupChannelForumFixture(t)

	posts := [][]string{
		{"channel", "post", "board", "opened", "--as", "main", "--thread", "t1", "--title", "第一主题", "--description", "主题描述", "--labels", "bug", "--assignees", "alice"},
		{"channel", "post", "board", "comment", "--as", "alice", "--thread", "t1", "--text", "收到，正在看"},
		{"channel", "post", "board", "status", "--as", "main", "--thread", "t1", "--status", "in-progress"},
		{"channel", "post", "board", "labels", "--as", "main", "--thread", "t1", "--labels", "bug,urgent"},
		{"channel", "post", "board", "assignees", "--as", "main", "--thread", "t1", "--assignees", "alice,bob"},
		{"channel", "post", "board", "summary", "--as", "main", "--thread", "t1", "--summary", "根因是空指针"},
		{"channel", "post", "board", "processed", "--as", "main", "--thread", "t1"},
	}
	for i, args := range posts {
		out, err := runChannelForumArgs(t, args...)
		if err != nil {
			t.Fatalf("第 %d 条 post 失败：%v", i, err)
		}
		var ev channel.ChannelEvent
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &ev); err != nil {
			t.Fatalf("第 %d 条 post 输出不是 JSON：%q %v", i, out, err)
		}
		if ev.Kind != "thread" {
			t.Fatalf("第 %d 条 post 事件 kind 应为 thread：%+v", i, ev)
		}
	}

	// forum 列表：含 t1，状态为 processed。
	out, err := runChannelForumArgs(t, "channel", "forum", "board")
	if err != nil {
		t.Fatalf("forum 列表失败：%v", err)
	}
	if !strings.Contains(out, "t1 [processed] 第一主题") {
		t.Errorf("forum 列表应含 t1 processed 行，得到：\n%s", out)
	}
	if !strings.Contains(out, "labels=bug,urgent") || !strings.Contains(out, "assignees=alice,bob") {
		t.Errorf("forum 列表应含标签与负责人，得到：\n%s", out)
	}

	// --status 过滤：processed 命中，open 不命中。
	out, err = runChannelForumArgs(t, "channel", "forum", "board", "--status", "processed")
	if err != nil || !strings.Contains(out, "t1") {
		t.Errorf("--status processed 应命中 t1：out=%q err=%v", out, err)
	}
	out, err = runChannelForumArgs(t, "channel", "forum", "board", "--status", "open")
	if err != nil || !strings.Contains(out, "暂无主题") {
		t.Errorf("--status open 应无主题：out=%q err=%v", out, err)
	}

	// --raw 输出 thread JSON。
	out, err = runChannelForumArgs(t, "channel", "forum", "board", "--raw")
	if err != nil {
		t.Fatalf("forum --raw 失败：%v", err)
	}
	var st channel.ThreadState
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &st); err != nil {
		t.Fatalf("forum --raw 输出不是 JSON：%q %v", out, err)
	}
	if st.ID != "t1" || st.Summary != "根因是空指针" || !st.Processed {
		t.Fatalf("forum --raw 内容不对：%+v", st)
	}

	// thread 时间线：含标题行与 7 条事件行。
	out, err = runChannelForumArgs(t, "channel", "thread", "board", "t1")
	if err != nil {
		t.Fatalf("thread 查看失败：%v", err)
	}
	for _, want := range []string{
		"t1 [processed] 第一主题", "描述：主题描述", "标签：bug,urgent",
		"负责人：alice,bob", "摘要：根因是空指针",
		"opened 发帖人=main", "comment 发帖人=alice 收到，正在看",
		"status 发帖人=main", "processed 发帖人=main",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("时间线应含 %q，得到：\n%s", want, out)
		}
	}

	// rename：改名后列表只有新名；用旧名仍能查到时间线。
	out, err = runChannelForumArgs(t, "channel", "thread", "rename", "board", "t1", "t1-new", "--as", "main")
	if err != nil {
		t.Fatalf("rename 失败：%v", err)
	}
	var rev channel.ChannelEvent
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &rev); err != nil || rev.Action != "rename" {
		t.Fatalf("rename 输出不对：%q %v", out, err)
	}
	out, err = runChannelForumArgs(t, "channel", "forum", "board")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "t1-new [processed]") {
		t.Errorf("改名后列表应含 t1-new，得到：\n%s", out)
	}
	if strings.Contains(out, "t1 [processed]") {
		t.Errorf("改名后列表不应再出现旧名 t1，得到：\n%s", out)
	}
	out, err = runChannelForumArgs(t, "channel", "thread", "board", "t1")
	if err != nil {
		t.Fatalf("用旧名查时间线失败：%v", err)
	}
	if !strings.Contains(out, "t1-new [processed]") || !strings.Contains(out, "rename 发帖人=main") {
		t.Errorf("旧名时间线应解析到新名并含改名事件，得到：\n%s", out)
	}
}

func TestChannelForumChatChannelErrors(t *testing.T) {
	setupChannelForumFixture(t)
	cases := [][]string{
		{"channel", "post", "talk", "opened", "--as", "main", "--thread", "t1"},
		{"channel", "forum", "talk"},
		{"channel", "thread", "talk", "t1"},
		{"channel", "thread", "rename", "talk", "t1", "t2", "--as", "main"},
	}
	for _, args := range cases {
		_, err := runChannelForumArgs(t, args...)
		if err == nil {
			t.Errorf("chat 频道上 %v 应报错，实际成功", args[1:])
			continue
		}
		if !strings.Contains(err.Error(), "forum") {
			t.Errorf("chat 频道上 %v 应报 forum 类型错误，得到：%v", args[1:], err)
		}
	}
}

func TestChannelForumValidation(t *testing.T) {
	setupChannelForumFixture(t)
	cases := [][]string{
		{"channel", "post", "board", "opened", "--thread", "t1"},                // 缺 --as
		{"channel", "post", "board", "rename", "--as", "main"},                  // post rename
		{"channel", "post", "board", "comment", "--as", "main", "--text", "hi"}, // 缺 --thread
		{"channel", "post", "board", "bogus", "--as", "main", "--thread", "t1"}, // 非法动作
		{"channel", "thread", "rename", "board", "t1", "t2"},                    // rename 缺 --as
		{"channel", "thread", "board", "ghost"},                                 // 不存在的主题
		{"channel", "forum", "ghost-channel"},                                   // 不存在的频道
	}
	for _, args := range cases {
		if _, err := runChannelForumArgs(t, args...); err == nil {
			t.Errorf("%v 应报错，实际成功", args)
		}
	}
}

func TestChannelForumTextFileAndStdin(t *testing.T) {
	fx := setupChannelForumFixture(t)
	bodyFile := filepath.Join(fx.proj, "body.txt")
	if err := os.WriteFile(bodyFile, []byte("文件正文\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runChannelForumArgs(t, "channel", "post", "board", "opened", "--as", "main", "--thread", "t1", "--title", "文件正文主题"); err != nil {
		t.Fatal(err)
	}
	out, err := runChannelForumArgs(t, "channel", "post", "board", "comment", "--as", "main", "--thread", "t1", "--text-file", bodyFile)
	if err != nil {
		t.Fatalf("--text-file 失败：%v", err)
	}
	var ev channel.ChannelEvent
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &ev); err != nil || ev.Text != "文件正文" {
		t.Fatalf("--text-file 正文不对：%q %+v", out, ev)
	}

	// --stdin：替换 os.Stdin 为管道。
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r
	go func() {
		_, _ = w.WriteString("标准输入正文\n")
		_ = w.Close()
	}()
	body, err := resolveTextBody("", "", true)
	os.Stdin = oldStdin
	_ = r.Close()
	if err != nil || body != "标准输入正文" {
		t.Fatalf("--stdin 正文不对：%q %v", body, err)
	}
}
