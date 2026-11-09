package lifecycle

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// stubTTY 把交互终端判定替换为替身，测试结束恢复。
func stubTTY(t *testing.T, value bool) {
	t.Helper()
	old := isInteractiveTerminal
	isInteractiveTerminal = func() bool { return value }
	t.Cleanup(func() { isInteractiveTerminal = old })
}

func lookPathGit() (string, error) {
	return exec.LookPath("git")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v 失败：%v", args, err)
	}
}

func TestAblateRestoreRoundTrip(t *testing.T) {
	dir := setupProject(t)
	// 用户自有文件（不在清单里）：切除后必须还在，恢复后仍在。
	userFile := filepath.Join(dir, ".claude", "skills", "custom", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(userFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userFile, []byte("# my skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 一个带执行权限的文件，验证恢复后权限也一致。
	execFile := filepath.Join(dir, ".sunshine", "scripts", "run.sh")
	if err := os.MkdirAll(filepath.Dir(execFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(execFile, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, dir)

	stateRoot := t.TempDir()
	t.Setenv(AblationStateRootEnv, stateRoot)

	if err := Ablate(dir, "0.1.0", AblateOptions{Yes: true}); err != nil {
		t.Fatalf("切除失败：%v", err)
	}
	for _, rel := range []string{".sunshine", ".agents"} {
		if _, err := os.Stat(filepath.Join(dir, rel)); !os.IsNotExist(err) {
			t.Errorf("切除后 %s 应已不存在", rel)
		}
	}
	// .cursor/hooks.json 是混合文件：只剩 version 键，清理后保留；
	// .claude/settings.json 同理（env 等键保留）。这是上游行为：
	// 清理器只剥离 Sunshine 字段，不删还有用户内容的壳。
	cursorHooks, err := os.ReadFile(filepath.Join(dir, ".cursor", "hooks.json"))
	if err != nil {
		t.Fatalf("切除后 .cursor/hooks.json 应保留为清理后的壳：%v", err)
	}
	if strings.Contains(string(cursorHooks), "sunshine hook") {
		t.Error("切除后 .cursor/hooks.json 里不应再有 Sunshine hook")
	}
	if _, err := os.Stat(userFile); err != nil {
		t.Errorf("切除后用户自有文件应还在：%v", err)
	}
	// 事务目录在项目外，含 state.json + backup/。
	paths, err := GetTransactionPaths(dir, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.StateFile); err != nil {
		t.Errorf("事务 state.json 应存在：%v", err)
	}
	if _, err := os.Stat(paths.BackupDir); err != nil {
		t.Errorf("事务 backup/ 应存在：%v", err)
	}
	tx, err := LoadAblationTransaction(dir)
	if err != nil {
		t.Fatal(err)
	}
	if tx == nil || tx.State.Status != AblationApplied {
		t.Fatalf("事务状态应为 applied，实际 %+v", tx)
	}
	if tx.State.SunshineVersion != "0.1.0" {
		t.Errorf("事务版本 = %q，想得到 0.1.0", tx.State.SunshineVersion)
	}

	// 再次切除应先提示 restore。
	if err := Ablate(dir, "0.1.0", AblateOptions{Yes: true}); err == nil || !strings.Contains(err.Error(), "sunshine restore") {
		t.Errorf("重复切除应提示先 restore，实际 err=%v", err)
	}

	// dry-run 恢复：不改动、不删事务。
	if err := Restore(dir, RestoreOptions{Yes: true, DryRun: true}); err != nil {
		t.Fatalf("dry-run 恢复失败：%v", err)
	}
	if _, err := os.Stat(paths.TransactionDir); err != nil {
		t.Error("dry-run 恢复不应删除事务目录")
	}
	if _, err := os.Stat(filepath.Join(dir, ".sunshine")); !os.IsNotExist(err) {
		t.Error("dry-run 恢复不应恢复任何文件")
	}

	// 真实恢复：逐字节（含权限）一致，事务目录删除。
	if err := Restore(dir, RestoreOptions{Yes: true}); err != nil {
		t.Fatalf("恢复失败：%v", err)
	}
	if _, err := os.Stat(paths.TransactionDir); !os.IsNotExist(err) {
		t.Error("恢复后事务目录应被删除")
	}
	after := snapshot(t, dir)
	if !reflect.DeepEqual(before, after) {
		t.Error("恢复后快照与切除前不一致")
		for rel, entry := range before {
			if got, ok := after[rel]; !ok {
				t.Errorf("恢复后缺少 %s", rel)
			} else if got != entry {
				t.Errorf("%s 不一致：切除前 %+v，恢复后 %+v", rel, entry, got)
			}
		}
		for rel := range after {
			if _, ok := before[rel]; !ok {
				t.Errorf("恢复后多出 %s", rel)
			}
		}
	}
}

func TestRestoreConflict(t *testing.T) {
	dir := setupProject(t)
	stateRoot := t.TempDir()
	t.Setenv(AblationStateRootEnv, stateRoot)
	if err := Ablate(dir, "0.1.0", AblateOptions{Yes: true}); err != nil {
		t.Fatal(err)
	}
	// 切除期间改动托管路径：重建一个内容不同的 settings.json。
	// 先记下切除后的形态（清理后的内容）：恢复前必须先回到
	// 该形态，删除文件是不够的——对混合文件而言，
	// 期望的切除后状态是清理后的文件，不是“不存在”
	// （上游 collectRestoreConflicts 同理）。
	settingsPath := filepath.Join(dir, ".claude", "settings.json")
	ablatedSettings, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settingsPath, []byte("{\"tampered\": true}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = Restore(dir, RestoreOptions{Yes: true})
	if err == nil {
		t.Fatal("切除期间被改动时恢复应被拒绝")
	}
	var conflictErr *AblationConflictError
	if !errors.As(err, &conflictErr) {
		t.Errorf("应返回 AblationConflictError，实际 %T：%v", err, err)
	}
	// 事务应还在（未删），且被改动的文件未被覆盖。
	paths, _ := GetTransactionPaths(dir, stateRoot)
	if _, err := os.Stat(paths.TransactionDir); err != nil {
		t.Error("冲突拒绝后事务目录应保留")
	}
	after, _ := os.ReadFile(settingsPath)
	if string(after) != "{\"tampered\": true}\n" {
		t.Error("冲突拒绝后被改动的文件不应被覆盖")
	}
	// 把改动清掉（恢复成切除后的形态）后恢复应成功。
	if err := os.WriteFile(settingsPath, ablatedSettings, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Restore(dir, RestoreOptions{Yes: true}); err != nil {
		t.Errorf("清掉改动后恢复应成功，实际：%v", err)
	}
}

func TestRestoreNoTransaction(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(AblationStateRootEnv, t.TempDir())
	if err := Restore(dir, RestoreOptions{Yes: true}); err != nil {
		t.Errorf("无事务时应直接提示、不报错，实际：%v", err)
	}
}

func TestAblateStateRootInsideProject(t *testing.T) {
	dir := setupProject(t)
	inside := filepath.Join(dir, ".ablations")
	t.Setenv(AblationStateRootEnv, inside)
	err := Ablate(dir, "0.1.0", AblateOptions{Yes: true})
	if err == nil || !strings.Contains(err.Error(), "必须指向被切除项目之外") {
		t.Errorf("事务根目录在项目内时应拒绝，实际 err=%v", err)
	}
}

func TestAblateNoManifest(t *testing.T) {
	dir := setupProject(t)
	if err := os.Remove(filepath.Join(dir, ".sunshine", ".template-hashes.json")); err != nil {
		t.Fatal(err)
	}
	t.Setenv(AblationStateRootEnv, t.TempDir())
	err := Ablate(dir, "0.1.0", AblateOptions{Yes: true})
	if err == nil || !strings.Contains(err.Error(), "清单") {
		t.Errorf("清单缺失时切除应拒绝，实际 err=%v", err)
	}
}

func TestAblateInteractiveCancel(t *testing.T) {
	stubTTY(t, true)
	dir := setupProject(t)
	t.Setenv(AblationStateRootEnv, t.TempDir())
	before := snapshot(t, dir)
	if err := Ablate(dir, "0.1.0", AblateOptions{
		Confirm: func(string) (bool, error) { return false, nil },
	}); err != nil {
		t.Fatalf("取消切除不应报错，实际：%v", err)
	}
	if after := snapshot(t, dir); !reflect.DeepEqual(before, after) {
		t.Error("取消切除后不应改动任何文件")
	}
}

func TestAblateDryRun(t *testing.T) {
	dir := setupProject(t)
	t.Setenv(AblationStateRootEnv, t.TempDir())
	before := snapshot(t, dir)
	if err := Ablate(dir, "0.1.0", AblateOptions{Yes: true, DryRun: true}); err != nil {
		t.Fatalf("dry-run 切除失败：%v", err)
	}
	if after := snapshot(t, dir); !reflect.DeepEqual(before, after) {
		t.Error("dry-run 切除不应改动任何文件")
	}
}
