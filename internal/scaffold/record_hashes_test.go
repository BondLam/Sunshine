package scaffold

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRecordHashesMergesIntoManifest(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir, "张三", false); err != nil {
		t.Fatalf("Init 失败：%v", err)
	}
	manifestPath := filepath.Join(dir, ".sunshine", ".template-hashes.json")
	before, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("读清单失败：%v", err)
	}
	var beforeManifest struct {
		Version int               `json:"__version"`
		Files   map[string]string `json:"files"`
	}
	if err := json.Unmarshal(before, &beforeManifest); err != nil {
		t.Fatalf("解析清单失败：%v", err)
	}
	if beforeManifest.Version != 2 {
		t.Errorf("清单版本不对：%d", beforeManifest.Version)
	}
	beforeCount := len(beforeManifest.Files)
	if beforeCount == 0 {
		t.Fatal("Init 写出的清单为空")
	}

	// 模拟平台安装写出的文件。
	rel := ".claude/settings.json"
	content := []byte("{\"hooks\": true}\n")
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, rel), content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RecordHashes(dir, []string{rel}); err != nil {
		t.Fatalf("RecordHashes 失败：%v", err)
	}
	after, _ := os.ReadFile(manifestPath)
	var afterManifest struct {
		Version int               `json:"__version"`
		Files   map[string]string `json:"files"`
	}
	if err := json.Unmarshal(after, &afterManifest); err != nil {
		t.Fatalf("解析新清单失败：%v", err)
	}
	if afterManifest.Version != 2 {
		t.Errorf("新清单版本不对：%d", afterManifest.Version)
	}
	// 已有条目保留。
	for k, v := range beforeManifest.Files {
		if afterManifest.Files[k] != v {
			t.Errorf("已有条目丢失或被改：%s", k)
		}
	}
	if len(afterManifest.Files) != beforeCount+1 {
		t.Errorf("条目数不对：原来 %d，现在 %d", beforeCount, len(afterManifest.Files))
	}
	sum := sha256.Sum256(content)
	if afterManifest.Files[rel] != hex.EncodeToString(sum[:]) {
		t.Errorf("新条目哈希不对：%q", afterManifest.Files[rel])
	}
	// 再次写入同一文件应更新哈希而非重复累积条目。
	if err := os.WriteFile(filepath.Join(dir, rel), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RecordHashes(dir, []string{rel}); err != nil {
		t.Fatalf("重复 RecordHashes 失败：%v", err)
	}
	again, _ := os.ReadFile(manifestPath)
	var againManifest struct {
		Files map[string]string `json:"files"`
	}
	if err := json.Unmarshal(again, &againManifest); err != nil {
		t.Fatal(err)
	}
	if len(againManifest.Files) != beforeCount+1 {
		t.Errorf("重复写入后条目数不对：%d", len(againManifest.Files))
	}
	sum2 := sha256.Sum256([]byte("changed"))
	if againManifest.Files[rel] != hex.EncodeToString(sum2[:]) {
		t.Error("重复写入后哈希未更新")
	}
}

func TestRecordHashesMissingFile(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir, "张三", false); err != nil {
		t.Fatalf("Init 失败：%v", err)
	}
	if err := RecordHashes(dir, []string{".claude/settings.json"}); err == nil {
		t.Error("不存在的文件应报错")
	}
}

func TestRecordHashesCreatesManifest(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".sunshine"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RecordHashes(dir, []string{"a.txt"}); err != nil {
		t.Fatalf("RecordHashes 失败：%v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".sunshine", ".template-hashes.json"))
	if err != nil {
		t.Fatalf("清单未生成：%v", err)
	}
	var manifest struct {
		Version int               `json:"__version"`
		Files   map[string]string `json:"files"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("解析清单失败：%v", err)
	}
	if manifest.Version != 2 || len(manifest.Files) != 1 {
		t.Errorf("清单内容不对：%+v", manifest)
	}
}
