package lifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// 项目外版本化恢复事务（对照 utils/ablation-store.ts）。
//
// 事务目录在项目外（SUNSHINE_ABLATION_STATE_ROOT，未设时默认
// ~/.sunshine/ablations/v1），内含 state.json + backup/。
// state.json 记录切除前每个托管路径的指纹与期望的切除后指纹，
// backup/ 存切除前内容的精确副本。恢复时先逐项预检冲突，
// 再按 state.json 精确恢复，成功后删除事务目录。

// AblationSchemaVersion 是事务 schema 版本（上游 ABLATION_SCHEMA_VERSION = 1）。
const AblationSchemaVersion = 1

// AblationCapability 是完整切除的能力标识（上游能力标识同名改写）。
const AblationCapability = "sunshine.full"

// AblationStateRootEnv 是事务根目录的环境变量名
// （对应上游同名环境变量）。
const AblationStateRootEnv = "SUNSHINE_ABLATION_STATE_ROOT"

// AblationStatus 是事务状态（preparing / applied / restoring / conflict）。
type AblationStatus string

const (
	AblationPreparing      AblationStatus = "preparing"
	AblationApplied        AblationStatus = "applied"
	AblationRestoring      AblationStatus = "restoring"
	AblationStatusConflict AblationStatus = "conflict"
)

// Fingerprint 是单个路径的指纹（不解开末级符号链接）。
type Fingerprint struct {
	Kind   string `json:"kind"` // absent | file | directory | symlink
	SHA256 string `json:"sha256,omitempty"`
	Size   int64  `json:"size,omitempty"`
	Mode   uint32 `json:"mode,omitempty"`
	Target string `json:"target,omitempty"`
}

// AblationEntry 是一条恢复事务条目。
type AblationEntry struct {
	RelativePath    string      `json:"relativePath"`
	Pre             Fingerprint `json:"pre"`
	ExpectedAblated Fingerprint `json:"expectedAblated"`
	BackupPath      string      `json:"backupPath,omitempty"`
}

// AblationState 是 state.json 的完整内容（schema v1）。
type AblationState struct {
	SchemaVersion       int               `json:"schemaVersion"`
	Status              AblationStatus    `json:"status"`
	ProjectRoot         string            `json:"projectRoot"`
	ProjectKey          string            `json:"projectKey"`
	SunshineVersion     string            `json:"sunshineVersion"`
	Capabilities        []string          `json:"capabilities"`
	CreatedAt           string            `json:"createdAt"`
	ConfiguredPlatforms []string          `json:"configuredPlatforms"`
	Manifest            map[string]string `json:"manifest"`
	Entries             []AblationEntry   `json:"entries"`
}

// TransactionPaths 是事务的全部路径。
type TransactionPaths struct {
	StateRoot      string
	TransactionDir string
	StateFile      string
	BackupDir      string
	LockFile       string
}

// LoadedAblationTransaction 是已加载的事务（路径 + 状态）。
type LoadedAblationTransaction struct {
	Paths TransactionPaths
	State AblationState
}

// AblationConflict 是一条恢复冲突：某托管路径在切除期间被改动。
type AblationConflict struct {
	RelativePath string
	Expected     Fingerprint
	Actual       Fingerprint
}

// AblationConflictError 是恢复因冲突被拒绝的错误。
type AblationConflictError struct {
	Conflicts []AblationConflict
}

func (e *AblationConflictError) Error() string {
	var paths []string
	for _, c := range e.Conflicts {
		paths = append(paths, c.RelativePath)
	}
	return fmt.Sprintf("恢复被拒绝：%d 个 Sunshine 托管路径在切除期间被改动：%s", len(e.Conflicts), strings.Join(paths, "、"))
}

// ---------------------------------------------------------------------------
// 指纹
// ---------------------------------------------------------------------------

func modeBits(info os.FileInfo) uint32 {
	return uint32(info.Mode().Perm())
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func sha256HexString(s string) string {
	return sha256Hex([]byte(s))
}

// hashDirectory 按上游 hashDirectory 算目录指纹：
// 子项按名排序，逐项写入（名 + \0 + 子指纹 JSON + \0）。
func hashDirectory(absPath string) (string, error) {
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return "", err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	h := sha256.New()
	for _, entry := range entries {
		child := filepath.Join(absPath, entry.Name())
		fp, err := FingerprintPath(child)
		if err != nil {
			return "", err
		}
		raw, err := json.Marshal(fp)
		if err != nil {
			return "", err
		}
		_, _ = h.Write([]byte(entry.Name()))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(raw)
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// FingerprintPath 给单个路径算指纹（不解开末级符号链接）。
func FingerprintPath(absPath string) (Fingerprint, error) {
	stat, err := lstatIfPresent(absPath)
	if err != nil {
		return Fingerprint{}, err
	}
	if stat == nil {
		return Fingerprint{Kind: "absent"}, nil
	}
	if stat.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(absPath)
		if err != nil {
			return Fingerprint{}, err
		}
		return Fingerprint{Kind: "symlink", Target: target, Mode: modeBits(stat)}, nil
	}
	if stat.Mode().IsRegular() {
		content, err := os.ReadFile(absPath)
		if err != nil {
			return Fingerprint{}, err
		}
		return Fingerprint{Kind: "file", SHA256: sha256Hex(content), Size: int64(len(content)), Mode: modeBits(stat)}, nil
	}
	if stat.IsDir() {
		sum, err := hashDirectory(absPath)
		if err != nil {
			return Fingerprint{}, err
		}
		return Fingerprint{Kind: "directory", SHA256: sum, Mode: modeBits(stat)}, nil
	}
	return Fingerprint{}, fmt.Errorf("切除范围内有不支持的文件系统对象：%s", absPath)
}

// ExpectedFileFingerprint 由内容与权限算期望的文件指纹。
func ExpectedFileFingerprint(content []byte, mode uint32) Fingerprint {
	return Fingerprint{Kind: "file", SHA256: sha256Hex(content), Size: int64(len(content)), Mode: mode}
}

// FingerprintsEqual 比较两个指纹是否相等。
func FingerprintsEqual(left, right Fingerprint) bool {
	if left.Kind != right.Kind {
		return false
	}
	switch left.Kind {
	case "absent":
		return true
	case "symlink":
		return left.Target == right.Target && left.Mode == right.Mode
	case "file":
		return left.SHA256 == right.SHA256 && left.Size == right.Size && left.Mode == right.Mode
	case "directory":
		return left.SHA256 == right.SHA256 && left.Mode == right.Mode
	}
	return false
}

// ---------------------------------------------------------------------------
// 事务根目录与路径
// ---------------------------------------------------------------------------

// CanonicalProjectRoot 返回项目根的规范路径（解析符号链接）。
func CanonicalProjectRoot(cwd string) string {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return cwd
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return abs
	}
	return real
}

// ProjectKey 由项目根算事务键（规范路径的 SHA-256 十六进制）。
func ProjectKey(projectRoot string) string {
	return sha256HexString(CanonicalProjectRoot(projectRoot))
}

// GetAblationStateRoot 返回事务根目录：环境变量优先，
// 未设时默认 ~/.sunshine/ablations/v1（上游默认位置同名改写）。
func GetAblationStateRoot() string {
	if override := strings.TrimSpace(os.Getenv(AblationStateRootEnv)); override != "" {
		abs, err := filepath.Abs(override)
		if err == nil {
			return abs
		}
		return override
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".sunshine", "ablations", "v1")
	}
	return filepath.Join(home, ".sunshine", "ablations", "v1")
}

// GetTransactionPaths 返回某项目的事务路径。
func GetTransactionPaths(projectRoot, stateRoot string) (TransactionPaths, error) {
	canonical := CanonicalProjectRoot(projectRoot)
	if err := AssertExternalStateRoot(canonical, stateRoot); err != nil {
		return TransactionPaths{}, err
	}
	key := ProjectKey(canonical)
	transactionDir := filepath.Join(stateRoot, key)
	return TransactionPaths{
		StateRoot:      stateRoot,
		TransactionDir: transactionDir,
		StateFile:      filepath.Join(transactionDir, "state.json"),
		BackupDir:      filepath.Join(transactionDir, "backup"),
		LockFile:       filepath.Join(stateRoot, key+".lock"),
	}, nil
}

// projectedCanonicalPath 解析路径已存在部分的符号链接，
// 不存在的尾部原样接回（用于判断 stateRoot 是否在项目内）。
func projectedCanonicalPath(absPath string) string {
	cursor := filepath.Clean(absPath)
	var missing []string
	for {
		if _, err := lstatIfPresent(cursor); err == nil {
			break
		}
		parent := filepath.Dir(cursor)
		if parent == cursor {
			break
		}
		missing = append([]string{filepath.Base(cursor)}, missing...)
		cursor = parent
	}
	real, err := filepath.EvalSymlinks(cursor)
	if err != nil {
		real = cursor
	}
	return filepath.Join(append([]string{real}, missing...)...)
}

// AssertExternalStateRoot 校验事务根目录在项目外，否则拒绝。
func AssertExternalStateRoot(projectRoot, stateRoot string) error {
	projected := projectedCanonicalPath(stateRoot)
	if withinProject(projectRoot, projected) {
		return fmt.Errorf("%s 必须指向被切除项目之外的目录", AblationStateRootEnv)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 状态校验与读写
// ---------------------------------------------------------------------------

var hex64Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// ParseAblationState 解析并校验一份 state.json 内容；
// 未知 schema 一律硬停（fail closed）。
func ParseAblationState(raw []byte) (AblationState, error) {
	var state AblationState
	if err := json.Unmarshal(raw, &state); err != nil {
		return AblationState{}, fmt.Errorf("切除事务状态无法解析：%w", err)
	}
	if err := ValidateAblationState(state); err != nil {
		return AblationState{}, err
	}
	return state, nil
}

// ValidateAblationState 校验事务状态的结构与条目一致性。
func ValidateAblationState(state AblationState) error {
	if state.SchemaVersion != AblationSchemaVersion {
		return fmt.Errorf("切除事务 schema 版本不受支持：%d", state.SchemaVersion)
	}
	switch state.Status {
	case AblationPreparing, AblationApplied, AblationRestoring, AblationStatusConflict:
	default:
		return fmt.Errorf("切除事务状态不受支持：%q", state.Status)
	}
	if state.ProjectRoot == "" {
		return fmt.Errorf("切除事务缺少 projectRoot")
	}
	if !hex64Pattern.MatchString(state.ProjectKey) {
		return fmt.Errorf("切除事务 projectKey 格式不对")
	}
	if state.SunshineVersion == "" {
		return fmt.Errorf("切除事务缺少 sunshineVersion")
	}
	if len(state.Capabilities) != 1 || state.Capabilities[0] != AblationCapability {
		return fmt.Errorf("切除事务能力标识不受支持：%v", state.Capabilities)
	}
	if _, err := time.Parse(time.RFC3339, state.CreatedAt); err != nil {
		return fmt.Errorf("切除事务创建时间格式不对：%q", state.CreatedAt)
	}
	if len(state.Entries) == 0 {
		return fmt.Errorf("切除事务没有条目")
	}
	seen := map[string]bool{}
	for key := range state.Manifest {
		if err := ValidateManagedRelativePath(key); err != nil {
			return err
		}
	}
	for _, entry := range state.Entries {
		if err := ValidateManagedRelativePath(entry.RelativePath); err != nil {
			return err
		}
		if seen[entry.RelativePath] {
			return fmt.Errorf("切除事务有重复条目：%s", entry.RelativePath)
		}
		seen[entry.RelativePath] = true
		if entry.BackupPath != "" {
			if err := ValidateManagedRelativePath(entry.BackupPath); err != nil {
				return err
			}
		}
		switch entry.Pre.Kind {
		case "absent":
			if entry.BackupPath != "" {
				return fmt.Errorf("不存在的切除条目不应有备份：%s", entry.RelativePath)
			}
		case "file", "directory", "symlink":
			if entry.BackupPath == "" {
				return fmt.Errorf("切除条目缺少备份：%s", entry.RelativePath)
			}
			if entry.BackupPath != "backup/"+entry.RelativePath {
				return fmt.Errorf("切除条目的备份路径与项目路径不符：%s", entry.RelativePath)
			}
		default:
			return fmt.Errorf("切除条目指纹类型不受支持：%s", entry.Pre.Kind)
		}
	}
	return nil
}

func ensurePrivateDirectory(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.Chmod(dir, 0o700)
}

func writeState(paths TransactionPaths, state AblationState) error {
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := writeFileAtomic(paths.StateFile, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	return os.Chmod(paths.StateFile, 0o600)
}

// StageAblationInput 是建事务的输入。
type StageAblationInput struct {
	ProjectRoot         string
	SunshineVersion     string
	ConfiguredPlatforms []string
	Manifest            map[string]string
	Entries             []AblationEntry
}

// StageAblationTransaction 建事务并校验完整备份，再原子发布为
// preparing 状态。调用方在改动项目之前调它；lockHeld 为真时
// 调用方已持有本项目的切除操作锁。
func StageAblationTransaction(input StageAblationInput, lockHeld bool) (*LoadedAblationTransaction, error) {
	projectRoot := CanonicalProjectRoot(input.ProjectRoot)
	stateRoot := GetAblationStateRoot()
	if err := AssertExternalStateRoot(projectRoot, stateRoot); err != nil {
		return nil, err
	}
	paths, err := GetTransactionPaths(projectRoot, stateRoot)
	if err != nil {
		return nil, err
	}
	entries := append([]AblationEntry(nil), input.Entries...)
	sort.Slice(entries, func(i, j int) bool {
		return strings.Count(entries[i].RelativePath, "/") < strings.Count(entries[j].RelativePath, "/")
	})
	state := AblationState{
		SchemaVersion:       AblationSchemaVersion,
		Status:              AblationPreparing,
		ProjectRoot:         projectRoot,
		ProjectKey:          ProjectKey(projectRoot),
		SunshineVersion:     input.SunshineVersion,
		Capabilities:        []string{AblationCapability},
		CreatedAt:           time.Now().UTC().Format(time.RFC3339Nano),
		ConfiguredPlatforms: append([]string(nil), input.ConfiguredPlatforms...),
		Manifest:            input.Manifest,
		Entries:             entries,
	}
	if state.ConfiguredPlatforms == nil {
		state.ConfiguredPlatforms = []string{}
	}
	if err := ValidateAblationState(state); err != nil {
		return nil, err
	}
	if !lockHeld {
		var staged *LoadedAblationTransaction
		err := WithAblationProjectLock(paths, func() error {
			var err error
			staged, err = StageAblationTransaction(input, true)
			return err
		})
		return staged, err
	}
	if st, _ := lstatIfPresent(paths.TransactionDir); st != nil {
		return nil, fmt.Errorf("该项目已存在切除事务，请先运行 sunshine restore")
	}
	if err := ensurePrivateDirectory(paths.StateRoot); err != nil {
		return nil, err
	}
	if err := AssertExternalStateRoot(projectRoot, paths.StateRoot); err != nil {
		return nil, err
	}
	tempDir := filepath.Join(paths.StateRoot, fmt.Sprintf(".%s.%d.%d.tmp", ProjectKey(projectRoot), os.Getpid(), time.Now().UnixMilli()))
	tempPaths := TransactionPaths{
		StateRoot:      paths.StateRoot,
		TransactionDir: tempDir,
		StateFile:      filepath.Join(tempDir, "state.json"),
		BackupDir:      filepath.Join(tempDir, "backup"),
		LockFile:       paths.LockFile,
	}
	if err := ensurePrivateDirectory(tempPaths.TransactionDir); err != nil {
		return nil, err
	}
	if err := ensurePrivateDirectory(tempPaths.BackupDir); err != nil {
		removeExternalTransactionDir(tempPaths.TransactionDir)
		return nil, err
	}
	stageErr := func() error {
		for _, entry := range state.Entries {
			source, err := AssertSafeManagedPath(projectRoot, entry.RelativePath)
			if err != nil {
				return err
			}
			actualPre, err := FingerprintPath(source)
			if err != nil {
				return err
			}
			if !FingerprintsEqual(actualPre, entry.Pre) {
				return fmt.Errorf("切除预检时项目路径发生变化：%s", entry.RelativePath)
			}
			if entry.BackupPath == "" {
				continue
			}
			destination := filepath.Join(tempPaths.TransactionDir, filepath.FromSlash(entry.BackupPath))
			if st, _ := lstatIfPresent(destination); st == nil {
				if err := copyPath(source, destination, true); err != nil {
					return err
				}
			}
			if err := verifyBackupEntry(tempPaths.TransactionDir, entry); err != nil {
				return err
			}
		}
		if err := writeState(tempPaths, state); err != nil {
			return err
		}
		return os.Rename(tempPaths.TransactionDir, paths.TransactionDir)
	}()
	if stageErr != nil {
		removeExternalTransactionDir(tempPaths.TransactionDir)
		return nil, stageErr
	}
	return &LoadedAblationTransaction{Paths: paths, State: state}, nil
}

// LoadAblationTransaction 加载某项目的切除事务；无事务时返回 nil。
// 事务根目录取环境变量（或默认位置）。
func LoadAblationTransaction(projectRoot string) (*LoadedAblationTransaction, error) {
	return LoadAblationTransactionWithRoot(projectRoot, GetAblationStateRoot())
}

// LoadAblationTransactionWithRoot 在指定的事务根目录下加载事务。
func LoadAblationTransactionWithRoot(projectRoot, stateRoot string) (*LoadedAblationTransaction, error) {
	canonical := CanonicalProjectRoot(projectRoot)
	if err := AssertExternalStateRoot(canonical, stateRoot); err != nil {
		return nil, err
	}
	paths, err := GetTransactionPaths(canonical, stateRoot)
	if err != nil {
		return nil, err
	}
	if st, _ := lstatIfPresent(paths.StateFile); st == nil {
		return nil, nil
	}
	raw, err := os.ReadFile(paths.StateFile)
	if err != nil {
		return nil, fmt.Errorf("切除事务状态无法读取，拒绝改动项目：%w", err)
	}
	state, err := ParseAblationState(raw)
	if err != nil {
		return nil, fmt.Errorf("切除事务状态无法读取，拒绝改动项目：%w", err)
	}
	if state.ProjectRoot != canonical || state.ProjectKey != ProjectKey(canonical) {
		return nil, fmt.Errorf("切除事务不属于当前项目")
	}
	tx := &LoadedAblationTransaction{Paths: paths, State: state}
	for _, entry := range state.Entries {
		if err := verifyBackupEntry(paths.TransactionDir, entry); err != nil {
			return nil, err
		}
	}
	return tx, nil
}

// TransitionAblationState 把事务转到新状态并写盘。
func TransitionAblationState(tx *LoadedAblationTransaction, status AblationStatus) error {
	next := tx.State
	next.Status = status
	if err := writeState(tx.Paths, next); err != nil {
		return err
	}
	tx.State = next
	return nil
}

func verifyBackupEntry(transactionDir string, entry AblationEntry) error {
	if entry.BackupPath == "" {
		return nil
	}
	backupAbs := filepath.Join(transactionDir, filepath.FromSlash(entry.BackupPath))
	actual, err := FingerprintPath(backupAbs)
	if err != nil {
		return err
	}
	if !FingerprintsEqual(actual, entry.Pre) {
		return fmt.Errorf("备份校验失败：%s", entry.RelativePath)
	}
	return nil
}

// VerifyAblatedState 校验每条事务条目当前都处于期望的切除后状态。
func VerifyAblatedState(tx *LoadedAblationTransaction) error {
	for _, entry := range tx.State.Entries {
		abs, err := AssertSafeManagedPath(tx.State.ProjectRoot, entry.RelativePath)
		if err != nil {
			return err
		}
		actual, err := FingerprintPath(abs)
		if err != nil {
			return err
		}
		if !FingerprintsEqual(actual, entry.ExpectedAblated) {
			return fmt.Errorf("切除后状态校验失败：%s", entry.RelativePath)
		}
	}
	return nil
}

// VerifyRestoredState 校验每条事务条目当前都恢复到了切除前状态。
func VerifyRestoredState(tx *LoadedAblationTransaction) error {
	for _, entry := range tx.State.Entries {
		abs, err := AssertSafeManagedPath(tx.State.ProjectRoot, entry.RelativePath)
		if err != nil {
			return err
		}
		actual, err := FingerprintPath(abs)
		if err != nil {
			return err
		}
		if !FingerprintsEqual(actual, entry.Pre) {
			return fmt.Errorf("恢复后状态校验失败：%s", entry.RelativePath)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// 项目锁（对照 ablation-store.ts 的 acquireProjectLock）
// ---------------------------------------------------------------------------

func acquireProjectLock(paths TransactionPaths) error {
	if err := ensurePrivateDirectory(paths.StateRoot); err != nil {
		return err
	}
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(paths.LockFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = f.WriteString(strconv.Itoa(os.Getpid()))
			_ = f.Close()
			return nil
		}
		if !os.IsExist(err) {
			return err
		}
		raw, err := os.ReadFile(paths.LockFile)
		if err != nil {
			continue
		}
		holderPid, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
		if holderPid != 0 && pidAlive(holderPid) {
			return fmt.Errorf("已有另一个 Sunshine 切除操作正在进行（进程 %d），请稍后重试", holderPid)
		}
		_ = os.Remove(paths.LockFile)
	}
	return fmt.Errorf("无法获取 Sunshine 切除操作锁，请重试")
}

func releaseProjectLock(paths TransactionPaths) {
	raw, err := os.ReadFile(paths.LockFile)
	if err != nil {
		return
	}
	if strings.TrimSpace(string(raw)) == strconv.Itoa(os.Getpid()) {
		_ = os.Remove(paths.LockFile)
	}
}

// WithAblationProjectLock 在持有本项目切除操作锁的情况下执行 operation。
func WithAblationProjectLock(paths TransactionPaths, operation func() error) error {
	if err := acquireProjectLock(paths); err != nil {
		return err
	}
	defer releaseProjectLock(paths)
	return operation()
}

// ---------------------------------------------------------------------------
// 恢复
// ---------------------------------------------------------------------------

func mayAlreadyBeRestored(state AblationState) bool {
	return state.Status == AblationPreparing || state.Status == AblationRestoring
}

// CollectRestoreConflicts 预检恢复冲突：当前状态既不是期望的
// 切除后状态、又不是（中断恢复时）已恢复的切除前状态，即冲突。
func CollectRestoreConflicts(tx *LoadedAblationTransaction) []AblationConflict {
	var conflicts []AblationConflict
	for _, entry := range tx.State.Entries {
		abs, err := AssertSafeManagedPath(tx.State.ProjectRoot, entry.RelativePath)
		if err != nil {
			conflicts = append(conflicts, AblationConflict{RelativePath: entry.RelativePath, Expected: entry.ExpectedAblated, Actual: Fingerprint{Kind: "absent"}})
			continue
		}
		current, err := FingerprintPath(abs)
		if err != nil {
			conflicts = append(conflicts, AblationConflict{RelativePath: entry.RelativePath, Expected: entry.ExpectedAblated, Actual: Fingerprint{Kind: "absent"}})
			continue
		}
		if FingerprintsEqual(current, entry.ExpectedAblated) {
			continue
		}
		if mayAlreadyBeRestored(tx.State) && FingerprintsEqual(current, entry.Pre) {
			continue
		}
		conflicts = append(conflicts, AblationConflict{RelativePath: entry.RelativePath, Expected: entry.ExpectedAblated, Actual: current})
	}
	return conflicts
}

func copyPath(source, destination string, privateParents bool) error {
	stat, err := os.Lstat(source)
	if err != nil {
		return err
	}
	parent := filepath.Dir(destination)
	if privateParents {
		if err := ensurePrivateDirectory(parent); err != nil {
			return err
		}
	} else if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	if stat.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(source)
		if err != nil {
			return err
		}
		return os.Symlink(target, destination)
	}
	if stat.Mode().IsRegular() {
		data, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		if err := writeFileAtomic(destination, data, os.FileMode(modeBits(stat))); err != nil {
			return err
		}
		return os.Chmod(destination, os.FileMode(modeBits(stat)))
	}
	if stat.IsDir() {
		if err := os.MkdirAll(destination, 0o700); err != nil {
			return err
		}
		entries, err := os.ReadDir(source)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyPath(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name()), privateParents); err != nil {
				return err
			}
		}
		return os.Chmod(destination, os.FileMode(modeBits(stat)))
	}
	return fmt.Errorf("切除范围内有不支持的文件系统对象：%s", source)
}

func removePath(absPath string) {
	_ = os.RemoveAll(absPath)
}

func makeExternalTreeRemovable(absPath string) {
	stat, err := lstatIfPresent(absPath)
	if err != nil || stat == nil || stat.Mode()&os.ModeSymlink != 0 {
		return
	}
	if stat.IsDir() {
		_ = os.Chmod(absPath, 0o700)
		entries, err := os.ReadDir(absPath)
		if err != nil {
			return
		}
		for _, entry := range entries {
			makeExternalTreeRemovable(filepath.Join(absPath, entry.Name()))
		}
	} else {
		_ = os.Chmod(absPath, 0o600)
	}
}

func removeExternalTransactionDir(transactionDir string) {
	makeExternalTreeRemovable(transactionDir)
	_ = os.RemoveAll(transactionDir)
}

// DeleteAblationTransaction 删除外部事务目录。
func DeleteAblationTransaction(tx *LoadedAblationTransaction) {
	removeExternalTransactionDir(tx.Paths.TransactionDir)
}

func restoreEntry(tx *LoadedAblationTransaction, entry AblationEntry, verifyExpectedState bool) error {
	destination, err := AssertSafeManagedPath(tx.State.ProjectRoot, entry.RelativePath)
	if err != nil {
		return err
	}
	current, err := FingerprintPath(destination)
	if err != nil {
		return err
	}
	if FingerprintsEqual(current, entry.Pre) {
		return nil
	}
	if verifyExpectedState && !FingerprintsEqual(current, entry.ExpectedAblated) {
		return &AblationConflictError{Conflicts: []AblationConflict{{RelativePath: entry.RelativePath, Expected: entry.ExpectedAblated, Actual: current}}}
	}
	if entry.Pre.Kind == "absent" {
		removePath(destination)
		return nil
	}
	if entry.BackupPath == "" {
		return fmt.Errorf("缺少备份路径：%s", entry.RelativePath)
	}
	source := filepath.Join(tx.Paths.TransactionDir, filepath.FromSlash(entry.BackupPath))

	switch entry.Pre.Kind {
	case "file":
		content, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		tmpPath := filepath.Join(filepath.Dir(destination), fmt.Sprintf(".%s.%d.%d.restore.tmp", filepath.Base(destination), os.Getpid(), rand.Int63()))
		if err := writeFileAtomic(tmpPath, content, 0o600); err != nil {
			return err
		}
		if err := os.Chmod(tmpPath, os.FileMode(entry.Pre.Mode)); err != nil {
			_ = os.Remove(tmpPath)
			return err
		}
		// 上游在期望切除后状态为 absent 时用硬链接建文件、
		// 否则用改名覆盖；两者在内容与权限上等价，
		// 这里统一用改名（已存在时原子替换）。
		if err := os.Rename(tmpPath, destination); err != nil {
			_ = os.Remove(tmpPath)
			return err
		}
		return nil
	case "symlink":
		if !verifyExpectedState {
			removePath(destination)
		}
		if err := os.Symlink(entry.Pre.Target, destination); err != nil {
			if os.IsExist(err) {
				actual, _ := FingerprintPath(destination)
				return &AblationConflictError{Conflicts: []AblationConflict{{RelativePath: entry.RelativePath, Expected: entry.ExpectedAblated, Actual: actual}}}
			}
			return err
		}
		return nil
	default: // directory（或其余形态）：整目录从备份拷回
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		tmpPath := filepath.Join(filepath.Dir(destination), fmt.Sprintf(".%s.%d.%d.restore.tmp", filepath.Base(destination), os.Getpid(), rand.Int63()))
		if err := copyPath(source, tmpPath, false); err != nil {
			removePath(tmpPath)
			return err
		}
		if !verifyExpectedState {
			removePath(destination)
		}
		if err := os.Rename(tmpPath, destination); err != nil {
			removePath(tmpPath)
			return err
		}
		return nil
	}
}

// RestoreTransactionFiles 按 state.json 恢复全部条目（不做预检，
// 调用方负责先调 CollectRestoreConflicts 或在回滚里用）。
// 浅路径先恢复（父目录先于子文件落盘）。
func RestoreTransactionFiles(tx *LoadedAblationTransaction, verifyExpectedState bool) error {
	ordered := append([]AblationEntry(nil), tx.State.Entries...)
	sort.Slice(ordered, func(i, j int) bool {
		return strings.Count(ordered[i].RelativePath, "/") < strings.Count(ordered[j].RelativePath, "/")
	})
	for _, entry := range ordered {
		if err := restoreEntry(tx, entry, verifyExpectedState); err != nil {
			return err
		}
	}
	return nil
}

// RestoreAblationTransaction 精确恢复切除事务。
// dryRun 只做冲突预检，不改状态、不写盘、不删事务；
// deleteAfterRestore 为真时恢复成功后删除外部事务目录。
// 恢复前有冲突时一律拒绝写入（零写盘）；
// 非 dryRun 且事务不是 preparing/restoring 时先把状态标为 conflict。
func RestoreAblationTransaction(tx *LoadedAblationTransaction, dryRun, deleteAfterRestore bool) error {
	projectRoot := CanonicalProjectRoot(tx.State.ProjectRoot)
	paths, err := GetTransactionPaths(projectRoot, tx.Paths.StateRoot)
	if err != nil {
		return err
	}
	return WithAblationProjectLock(paths, func() error {
		reloaded, err := LoadAblationTransactionWithRoot(projectRoot, tx.Paths.StateRoot)
		if err != nil {
			return err
		}
		if reloaded == nil {
			return fmt.Errorf("切除事务在恢复开始前消失了")
		}
		tx.Paths = reloaded.Paths
		tx.State = reloaded.State
		conflicts := CollectRestoreConflicts(tx)
		if len(conflicts) > 0 {
			if !dryRun && !mayAlreadyBeRestored(tx.State) {
				_ = TransitionAblationState(tx, AblationStatusConflict)
			}
			return &AblationConflictError{Conflicts: conflicts}
		}
		if dryRun {
			return nil
		}
		if err := TransitionAblationState(tx, AblationRestoring); err != nil {
			return err
		}
		if err := RestoreTransactionFiles(tx, true); err != nil {
			return err
		}
		if err := VerifyRestoredState(tx); err != nil {
			return err
		}
		if deleteAfterRestore {
			DeleteAblationTransaction(tx)
		}
		return nil
	})
}

// RollbackAblationTransaction 回滚本次未完成的切除：
// 不用常规恢复预检（父目录可能暂时两头指纹都不合），
// 直接按备份恢复全部条目，校验后删除事务。
// lockHeld 为真时调用方已持有本项目的切除操作锁。
func RollbackAblationTransaction(tx *LoadedAblationTransaction, lockHeld bool) error {
	projectRoot := CanonicalProjectRoot(tx.State.ProjectRoot)
	paths, err := GetTransactionPaths(projectRoot, tx.Paths.StateRoot)
	if err != nil {
		return err
	}
	rollback := func() error {
		reloaded, err := LoadAblationTransactionWithRoot(projectRoot, tx.Paths.StateRoot)
		if err != nil {
			return err
		}
		if reloaded == nil {
			return fmt.Errorf("切除事务在回滚开始前消失了")
		}
		tx.Paths = reloaded.Paths
		tx.State = reloaded.State
		if err := RestoreTransactionFiles(tx, false); err != nil {
			return err
		}
		if err := VerifyRestoredState(tx); err != nil {
			return err
		}
		DeleteAblationTransaction(tx)
		return nil
	}
	if lockHeld {
		return rollback()
	}
	return WithAblationProjectLock(paths, rollback)
}
