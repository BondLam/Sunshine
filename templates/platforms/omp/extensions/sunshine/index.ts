import type { ExtensionAPI } from "@oh-my-pi/pi-coding-agent";
import { closeSync, existsSync, fstatSync, lstatSync, openSync, readFileSync, readdirSync, realpathSync, statSync, readSync } from "node:fs";
import { join, dirname, basename, isAbsolute, relative, resolve } from "node:path";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";

// ---------------------------------------------------------------------------
// 项目根目录检测
// ---------------------------------------------------------------------------

function findProjectRoot(startDir: string): string | null {
   let current = startDir;
   while (true) {
      if (existsSync(join(current, ".sunshine"))) return current;
      const parent = dirname(current);
      if (parent === current) break;
      current = parent;
   }
   return null;
}

// ---------------------------------------------------------------------------
// 会话身份工具函数（逻辑与 sunshine hook 的键清理/哈希/上下文键一致）
// ---------------------------------------------------------------------------

function sanitizeKey(raw: string): string {
   const safe = raw.trim().replace(/[^A-Za-z0-9._-]+/g, "_").replace(/^[._-]+|[._-]+$/g, "");
   return safe ? safe.slice(0, 160) : "";
}

function hashValue(raw: string): string {
   return createHash("sha256").update(raw).digest("hex").slice(0, 24);
}

function buildContextKey(platformName: string, kind: string, value: string): string {
   if (kind === "transcript") {
      return `${platformName}_transcript_${hashValue(value)}`;
   }
   const safeValue = sanitizeKey(value);
   return safeValue ? `${platformName}_${safeValue}` : `${platformName}_${hashValue(value)}`;
}

function deriveContextKey(ctx?: { sessionManager?: { getSessionId?: () => string | undefined; getSessionFile?: () => string | undefined } }): string | null {
   const sessionId = ctx?.sessionManager?.getSessionId?.();
   if (sessionId) {
      return buildContextKey("omp", "session", sessionId);
   }
   const sessionFile = ctx?.sessionManager?.getSessionFile?.();
   if (sessionFile) {
      return buildContextKey("omp", "transcript", sessionFile);
   }
   const override = process.env.SUNSHINE_CONTEXT_ID?.trim();
   return override ? sanitizeKey(override) || hashValue(override) : null;
}

function isInsideRoot(root: string, candidate: string): boolean {
   const rel = relative(root, candidate);
   return rel === "" || (rel !== ".." && !rel.startsWith("../") && !rel.startsWith("..\\") && !isAbsolute(rel));
}

// ---------------------------------------------------------------------------
// 受信任的上下文根目录（对应 Sunshine channel 的上下文信任逻辑；
// 独立副本，模板不从 CLI 包 import）。
// ---------------------------------------------------------------------------

const AUTO_TRUST_ENTRIES = ["tasks", "workspace"];

function stripTrustValue(s: string): string {
   return s.trim().replace(/\s*#.*$/, "").trim().replace(/^['"]|['"]$/g, "");
}

function parseChannelTrustSection(content: string): { trustedDirs: string[]; autoTrustSymlinks?: boolean } {
   const lines = content.split("\n");
   const trustedDirs: string[] = [];
   let autoTrustSymlinks: boolean | undefined;
   let inChannel = false;
   let inList = false;

   for (const raw of lines) {
      const line = raw.replace(/\r$/, "");
      const trimmed = line.trimEnd();
      if (trimmed.trim().startsWith("#")) continue;

      if (/^channel:\s*$/.test(trimmed)) {
         inChannel = true;
         inList = false;
         continue;
      }
      if (!inChannel) continue;

      if (trimmed.trim() !== "" && /^\S/.test(line)) {
         inChannel = false;
         inList = false;
         continue;
      }
      if (trimmed.trim() === "") continue;

      if (inList) {
         const item = trimmed.match(/^ {4}-\s*(.+)$/);
         if (item) {
            const val = stripTrustValue(item[1]!);
            if (val) trustedDirs.push(val);
            continue;
         }
         inList = false;
      }

      if (/^ {2}trusted_context_dirs:\s*$/.test(trimmed)) {
         inList = true;
         continue;
      }

      const boolMatch = trimmed.match(/^ {2}auto_trust_sunshine_symlinks:\s*(.+)$/);
      if (boolMatch) {
         const val = stripTrustValue(boolMatch[1]!).toLowerCase();
         if (val === "false") autoTrustSymlinks = false;
         else if (val === "true") autoTrustSymlinks = true;
         else process.stderr.write(`[channel] channel.auto_trust_sunshine_symlinks：值 '${val}' 无效，已忽略\n`);
         continue;
      }
   }

   return { trustedDirs, autoTrustSymlinks };
}

function resolveTrustedRoots(projectRoot: string): string[] {
   const configPath = join(projectRoot, ".sunshine", "config.yaml");
   let config: { trustedDirs: string[]; autoTrustSymlinks?: boolean } = { trustedDirs: [] };
   if (existsSync(configPath)) {
      try {
         config = parseChannelTrustSection(readFileSync(configPath, "utf-8"));
      } catch {
         // 忽略
      }
   }

   const roots: string[] = [];
   for (const entry of config.trustedDirs) {
      try {
         roots.push(realpathSync(resolve(projectRoot, entry)));
      } catch {
         // 条目不存在或无效——跳过
      }
   }

   if (config.autoTrustSymlinks !== false) {
      for (const entryName of AUTO_TRUST_ENTRIES) {
         const entryPath = join(projectRoot, ".sunshine", entryName);
         try {
            if (lstatSync(entryPath).isSymbolicLink()) {
               roots.push(realpathSync(entryPath));
            }
         } catch {
            // 缺失/损坏的符号链接——无可信任内容
         }
      }
   }

   return [...new Set(roots)];
}

function resolveProjectFile(
   projectRoot: string,
   file: string,
   trustedRoots: string[],
): string | null {
   try {
      const rootReal = realpathSync(projectRoot);
      const targetReal = realpathSync(resolve(projectRoot, file));
      if (isInsideRoot(rootReal, targetReal)) return targetReal;
      if (trustedRoots.some((root) => isInsideRoot(root, targetReal))) return targetReal;
      return null;
   } catch {
      return null;
   }
}

function displayProjectPath(projectRoot: string, filePath: string, taskDir?: string): string {
   const direct = relative(projectRoot, filePath).split("\\").join("/");
   if (direct && !direct.startsWith("../") && !isAbsolute(direct)) return direct;
   if (taskDir) {
      const taskRelative = relative(projectRoot, taskDir).split("\\").join("/");
      const taskLabel = taskRelative && !taskRelative.startsWith("../") && !isAbsolute(taskRelative)
         ? taskRelative
         : `.sunshine/tasks/${basename(taskDir)}`;
      const withinTask = relative(taskDir, filePath).split("\\").join("/");
      if (withinTask && !withinTask.startsWith("../") && !isAbsolute(withinTask)) {
         return `${taskLabel}/${withinTask}`;
      }
   }
   try {
      return relative(realpathSync(projectRoot), realpathSync(filePath)).split("\\").join("/");
   } catch {
      return relative(projectRoot, filePath).split("\\").join("/");
   }
}

// ---------------------------------------------------------------------------
// 当前任务解析
// ---------------------------------------------------------------------------

function resolveActiveTaskStatus(
   projectRoot: string,
   contextKey: string | null,
): { status: string; taskDir: string | null; taskTitle: string | null } {
   const sessionsDir = join(projectRoot, ".sunshine", ".runtime", "sessions");
   if (!existsSync(sessionsDir)) return { status: "no_task", taskDir: null, taskTitle: null };

   // --- 通过 context key 解析 session 文件 ---
   let sessionFilePath: string | null = null;

   if (contextKey) {
      const candidate = join(sessionsDir, `${contextKey}.json`);
      if (existsSync(candidate)) {
         sessionFilePath = candidate;
      } else {
         return { status: "no_task", taskDir: null, taskTitle: null };
      }
   } else {
      // 没有身份标识：只有恰好一个会话文件时才用单会话兜底。
      let sessionFiles: string[];
      try {
         sessionFiles = readdirSync(sessionsDir).filter((f) => f.endsWith(".json"));
      } catch {
         return { status: "no_task", taskDir: null, taskTitle: null };
      }
      if (sessionFiles.length === 1) {
         sessionFilePath = join(sessionsDir, sessionFiles[0]);
      } else {
         return { status: "no_task", taskDir: null, taskTitle: null };
      }
   }

   // --- 读取 session 数据 ---
   let sessionData: Record<string, unknown>;
   try {
      sessionData = JSON.parse(readFileSync(sessionFilePath, "utf-8"));
   } catch {
      return { status: "no_task", taskDir: null, taskTitle: null };
   }

   const currentTask = sessionData.current_task;
   if (typeof currentTask !== "string" || !currentTask)
      return { status: "no_task", taskDir: null, taskTitle: null };

   // 与下面 jsonl 引用文件走的同一个隔离区。`sunshine task`
   // 现在拒绝保存离开项目的引用，但修复前写入的会话文件仍可能
   // 持有这样的引用，而 `sunshine update` 不会重写
   // 会话文件——于是被污染的指针比关闭写入方的升级活得更久。

   const taskDir = resolveProjectFile(projectRoot, currentTask, resolveTrustedRoots(projectRoot));
   if (!taskDir) return { status: "no_task", taskDir: null, taskTitle: null };
   const taskJsonPath = join(taskDir, "task.json");
   if (!existsSync(taskJsonPath)) return { status: "no_task", taskDir: null, taskTitle: null };

   let taskData: Record<string, unknown>;
   try {
      taskData = JSON.parse(readFileSync(taskJsonPath, "utf-8"));
   } catch {
      return { status: "no_task", taskDir: null, taskTitle: null };
   }

   return {
      status: typeof taskData.status === "string" ? taskData.status : "planning",
      taskDir,
      taskTitle: typeof taskData.title === "string" ? taskData.title : null,
   };
}

// ---------------------------------------------------------------------------
// Session context — 调用 sunshine task context 默认模式（与 Claude hook 相同）
// ---------------------------------------------------------------------------

const SESSION_CONTEXT_TIMEOUT_MS = 5000;

function buildSessionContext(projectRoot: string, contextKey: string | null): string {
   try {
      const result = spawnSync("sunshine", ["task", "context"], {
         cwd: projectRoot,
         encoding: "utf-8",
         env: contextKey
            ? { ...process.env, SUNSHINE_CONTEXT_ID: contextKey }
            : process.env,
         timeout: SESSION_CONTEXT_TIMEOUT_MS,
         windowsHide: true,
      });
      if (result.status !== 0 || !result.stdout?.trim()) {
         return "";
      }
      return `<session-context>\n${result.stdout.trim()}\n</session-context>`;
   } catch {
      return "";
   }
}

// ---------------------------------------------------------------------------
// 任务上下文——prd.md、info.md 与 jsonl 引用的规范/调研文件
// ---------------------------------------------------------------------------

type AgentType = "sunshine-implement" | "sunshine-check" | "sunshine-research" | null;

function taskContextJsonlNames(agentType?: AgentType): string[] {
   if (agentType === "sunshine-implement") return ["implement.jsonl"];
   if (agentType === "sunshine-check") return ["check.jsonl"];
   if (agentType === "sunshine-research") return [];
   return ["implement.jsonl", "check.jsonl"];
}

function taskContextInputPaths(projectRoot: string, taskDir: string, agentType?: AgentType): string[] {
   const trustedRoots = resolveTrustedRoots(projectRoot);
   const paths = new Set<string>([
      join(projectRoot, ".sunshine", "config.yaml"),
      join(taskDir, "prd.md"),
      join(taskDir, "info.md"),
   ]);
   for (const jsonlName of taskContextJsonlNames(agentType)) {
      const jsonlPath = join(taskDir, jsonlName);
      paths.add(jsonlPath);
      if (!existsSync(jsonlPath)) continue;
      const displayPath = displayProjectPath(projectRoot, jsonlPath, taskDir);
      const { lines } = readJsonlLines(jsonlPath, displayPath);
      for (const line of lines) {
         try {
            const row = JSON.parse(line.trim()) as Record<string, unknown>;
            const file = typeof row.file === "string" ? row.file.trim() : "";
            const candidatePath = file ? resolve(projectRoot, file) : "";
            if (candidatePath && isInsideRoot(resolve(projectRoot), candidatePath)) paths.add(candidatePath);
            const targetPath = file ? resolveProjectFile(projectRoot, file, trustedRoots) : null;
            if (targetPath) paths.add(targetPath);
         } catch {
            // 种子行与格式错误的行不产生引用文件。
         }
      }
   }
   return [...paths];
}

function taskContextSignature(projectRoot: string, taskDir: string, agentType?: AgentType): string {
   return taskContextInputPaths(projectRoot, taskDir, agentType).map((filePath) => {
      try {
         const stat = statSync(filePath);
         return `${filePath}:${stat.mtimeMs}:${stat.ctimeMs}:${stat.size}`;
      } catch {
         return `${filePath}:missing`;
      }
   }).join("\n");
}

interface ContextInjectionLimits {
   max_file_bytes: number;
   max_artifact_bytes: number;
   max_total_bytes: number;
}

const DEFAULT_CONTEXT_INJECTION_LIMITS: ContextInjectionLimits = {
   max_file_bytes: 32768,
   max_artifact_bytes: 65536,
   max_total_bytes: 131072,
};
const MAX_JSONL_BYTES = 1024 * 1024;

function stripInlineComment(value: string): string {
   let quote: string | null = null;
   for (let i = 0; i < value.length; i++) {
      const char = value[i];
      if (quote) {
         if (char === quote) quote = null;
         continue;
      }
      if (char === "'" || char === '"') {
         quote = char;
         continue;
      }
      if (char === "#" && (i === 0 || /\s/.test(value[i - 1]!))) {
         return value.slice(0, i);
      }
   }
   return value;
}

function unquoteYaml(value: string): string {
   return value.length >= 2 && value[0] === value[value.length - 1] &&
      (value[0] === "'" || value[0] === '"')
      ? value.slice(1, -1)
      : value;
}

function readContextInjectionLimits(projectRoot: string): ContextInjectionLimits {
   const limits = { ...DEFAULT_CONTEXT_INJECTION_LIMITS };
   let config = "";
   try { config = readFileSync(join(projectRoot, ".sunshine", "config.yaml"), "utf-8"); } catch { return limits; }

   let inSection = false;
   let sectionIndent = -1;
   for (const rawLine of config.split(/\r?\n/)) {
      const trimmed = rawLine.trim();
      if (!inSection) {
         if (/^context_injection\s*:\s*(#.*)?$/.test(trimmed)) {
            inSection = true;
            sectionIndent = rawLine.length - rawLine.trimStart().length;
         }
         continue;
      }
      if (!trimmed || trimmed.startsWith("#")) continue;
      const indent = rawLine.length - rawLine.trimStart().length;
      if (indent <= sectionIndent) break;
      const match = trimmed.match(/^([A-Za-z_][A-Za-z0-9_]*)\s*:\s*(.*)$/);
      if (!match || !(match[1] in limits)) continue;
      const rawValue = unquoteYaml(stripInlineComment(match[2]!).trim()).trim();
      if (!/^\d+$/.test(rawValue)) continue;
      (limits as unknown as Record<string, number>)[match[1]!] = Number.parseInt(rawValue, 10);
   }
   return limits;
}

class ContextBudget {
   private totalBytesUsed: number;
   private terminalSummaryEmitted = false;
   private stopped = false;
   constructor(private readonly maxTotalBytes: number, initialUsed = 0) {
      this.totalBytesUsed = initialUsed;
   }

   get used(): number {
      return this.totalBytesUsed;
   }

   hasRoom(bytes: number): boolean {
      return !this.stopped && (this.maxTotalBytes <= 0 || this.totalBytesUsed + bytes <= this.maxTotalBytes);
   }

   emit(text: string): string | null {
      const bytes = Buffer.byteLength(text, "utf-8");
      if (!this.hasRoom(bytes)) return null;
      this.totalBytesUsed += bytes;
      return text;
   }

   emitCandidate(primary: string | null, fallback: string | null): string | null {
      if (primary !== null) {
         const direct = this.emit(primary);
         if (direct !== null) return direct;
      }
      if (fallback !== null) {
         const notice = this.emit(fallback);
         if (notice !== null) return notice;
      }
      return this.emitTerminalSummary();
   }

   private emitTerminalSummary(): string | null {
      if (this.terminalSummaryEmitted) return null;
      this.terminalSummaryEmitted = true;
      this.stopped = true;
      const summary = "[Sunshine：已达上下文上限；省略的条目在磁盘上仍为准，required_read 路径见上。]";
      if (this.maxTotalBytes <= 0) {
         this.totalBytesUsed += Buffer.byteLength(summary, "utf-8");
         return summary;
      }
      const remaining = this.maxTotalBytes - this.totalBytesUsed;
      if (remaining <= 0) return null;
      const bounded = truncateUtf8(Buffer.from(summary, "utf-8"), remaining).toString("utf-8");
      this.totalBytesUsed += Buffer.byteLength(bounded, "utf-8");
      return bounded;
   }
}

type Utf8Status = "valid" | "incomplete" | "invalid";

function utf8Status(data: Buffer): Utf8Status {
   for (let index = 0; index < data.length; index++) {
      const lead = data[index]!;
      if (lead <= 0x7f) continue;

      let length: number;
      let secondMin = 0x80;
      let secondMax = 0xbf;
      if (lead >= 0xc2 && lead <= 0xdf) length = 2;
      else if (lead >= 0xe0 && lead <= 0xef) {
         length = 3;
         if (lead === 0xe0) secondMin = 0xa0;
         if (lead === 0xed) secondMax = 0x9f;
      } else if (lead >= 0xf0 && lead <= 0xf4) {
         length = 4;
         if (lead === 0xf0) secondMin = 0x90;
         if (lead === 0xf4) secondMax = 0x8f;
      } else return "invalid";

      if (index + length > data.length) {
         for (let tail = index + 1; tail < data.length; tail++) {
            const min = tail === index + 1 ? secondMin : 0x80;
            const max = tail === index + 1 ? secondMax : 0xbf;
            if (data[tail]! < min || data[tail]! > max) return "invalid";
         }
         return "incomplete";
      }
      const second = data[index + 1]!;
      if (second < secondMin || second > secondMax) return "invalid";
      for (let continuation = index + 2; continuation < index + length; continuation++) {
         const byte = data[continuation]!;
         if (byte < 0x80 || byte > 0xbf) return "invalid";
      }
      index += length - 1;
   }
   return "valid";
}

function truncateUtf8(data: Buffer, cap: number): Buffer {
   if (cap <= 0 || data.length <= cap) return data;
   if (utf8Status(data.subarray(0, cap)) === "valid") return data.subarray(0, cap);
   // 有界读取已验证为完整的 UTF-8 流。
   // 只有它末尾不完整的码点可能让截断前缀无效。
   for (let end = cap - 1; end >= Math.max(0, cap - 4); end--) {
      const candidate = data.subarray(0, end);
      if (utf8Status(candidate) === "valid") return candidate;
   }
   return Buffer.alloc(0);
}

function isUtf8(data: Buffer): boolean {
   return utf8Status(data) === "valid";
}

function readFilePrefix(filePath: string, maxBytes: number): { data: Buffer; size: number } | null {
   let fd: number | null = null;
   try {
      fd = openSync(filePath, "r");
      if (maxBytes <= 0) {
         const data = readFileSync(fd);
         return { data, size: fstatSync(fd).size };
      }
      // 多读几个字节，让跨越上限的 UTF-8 序列能在
      // truncateUtf8 去掉其不完整后缀之前完成验证。如果路径被替换或变大，
      // 描述符仍保证读取有界。
      const data = Buffer.allocUnsafe(maxBytes + 4);
      const bytesRead = readFully(fd, data);
      return { data: data.subarray(0, bytesRead), size: fstatSync(fd).size };
   } catch {
      return null;
   } finally {
      if (fd !== null) closeSync(fd);
   }
}

function readFully(fd: number, buffer: Buffer): number {
   let total = 0;
   while (total < buffer.length) {
      const bytesRead = readSync(fd, buffer, total, buffer.length - total, total);
      if (bytesRead === 0) break;
      total += bytesRead;
   }
   return total;
}

function omittedNotice(file: string, size: number | null, reason: string): string {
   const sizeText = size === null ? "未知字节数" : `${size} 字节`;
   return `### ${file} [omitted]\n\n[Sunshine：已省略（${reason}）——${file}（${sizeText}）；required_read：${file}]`;
}

interface MaterializedFile {
   block: string | null;
   notice: string;
}

function materialize(
   targetPath: string,
   displayPath: string,
   reason: string,
   maxBytes: number,
   kind: "file" | "artifact",
): MaterializedFile {
   const file = readFilePrefix(targetPath, maxBytes);
   if (!file) {
      return { block: null, notice: omittedNotice(displayPath, null, `${kind} 缺失或无法读取`) };
   }
   const { data, size } = file;
   const encodingStatus = utf8Status(data);
   if (data.includes(0) || encodingStatus === "invalid" || (encodingStatus === "incomplete" && size <= data.length)) {
      return { block: null, notice: omittedNotice(displayPath, size, `二进制或非 UTF-8 的 ${kind}：${reason}`) };
   }
   const truncated = truncateUtf8(data, maxBytes);
   let content = truncated.toString("utf-8");
   let status: "inline" | "truncated" = "inline";
   if (truncated.length < size) {
      status = "truncated";
      content += `\n[Sunshine：已在 ${maxBytes} 字节处截断——请读 ${displayPath} 查看完整内容]`;
   }
   return {
      block: `### ${displayPath} [${status}]\n\n${content}`,
      notice: omittedNotice(displayPath, size, `已达总上下文上限：${reason}`),
   };
}

function readJsonlLines(jsonlPath: string, displayPath: string): { lines: string[]; omitted: string | null } {
   let fd: number | null = null;
   try {
      fd = openSync(jsonlPath, "r");
      const data = Buffer.allocUnsafe(MAX_JSONL_BYTES + 1);
      const bytesRead = readFully(fd, data);
      const size = fstatSync(fd).size;
      if (bytesRead > MAX_JSONL_BYTES || size > MAX_JSONL_BYTES) {
         return {
            lines: [],
            omitted: omittedNotice(displayPath, size, `清单超过 ${MAX_JSONL_BYTES} 字节解析上限`),
         };
      }
      const content = data.subarray(0, bytesRead);
      if (!isUtf8(content)) {
         return { lines: [], omitted: omittedNotice(displayPath, size, "清单不是有效的 UTF-8") };
      }
      return { lines: content.toString("utf-8").split(/\r?\n/), omitted: null };
   } catch {
      return { lines: [], omitted: omittedNotice(displayPath, null, "清单缺失或无法读取") };
   } finally {
      if (fd !== null) closeSync(fd);
   }
}

function buildTaskContext(projectRoot: string, taskDir: string, agentType?: AgentType): string {
   const parts: string[] = [];
   // 每次调用解析一次（而不是每个引用文件解析一次），避免
   // 为每个 jsonl 行重复解析 config.yaml。
   const trustedRoots = resolveTrustedRoots(projectRoot);
   const limits = readContextInjectionLimits(projectRoot);
   const prefix = "<task-context>\n上下文受 .sunshine/config.yaml 限制。标记为 [truncated] 或 [omitted] 的文件以磁盘为准；在使用缺失细节之前，先走它们的 required_read 路径。\n\n";
   const suffix = "\n</task-context>";
   const wrapperBytes = Buffer.byteLength(prefix + suffix, "utf-8");
   if (limits.max_total_bytes > 0 && limits.max_total_bytes < wrapperBytes) return "";
   const budget = new ContextBudget(limits.max_total_bytes, wrapperBytes);

   const appendCandidate = (primary: string | null, fallback: string | null = null): void => {
      const separator = parts.length > 0 ? "\n\n" : "";
      const output = budget.emitCandidate(
         primary === null ? null : separator + primary,
         fallback === null ? null : separator + fallback,
      );
      if (output !== null) parts.push(output);
   };

   // prd.md 与 info.md——始终包含
   const prdPath = join(taskDir, "prd.md");
   const relativePrdPath = displayProjectPath(projectRoot, prdPath, taskDir);
   if (existsSync(prdPath)) {
      const artifact = materialize(prdPath, relativePrdPath, "需求文档", limits.max_artifact_bytes, "artifact");
      appendCandidate(artifact.block, artifact.notice);
   }
   const infoPath = join(taskDir, "info.md");
   const relativeInfoPath = displayProjectPath(projectRoot, infoPath, taskDir);
   if (existsSync(infoPath)) {
      const artifact = materialize(infoPath, relativeInfoPath, "任务信息", limits.max_artifact_bytes, "artifact");
      appendCandidate(artifact.block, artifact.notice);
   }

   // 按 agent 类型确定读取哪些 jsonl 文件
   const jsonlNames = taskContextJsonlNames(agentType);

   // 一个文件可能被两个清单同时引用。用解析后的真实路径，
   // 让相对别名与符号链接路径不会重复占用上下文预算。
   const includedPaths = new Set<string>();

   for (const jsonlName of jsonlNames) {
      const jsonlPath = join(taskDir, jsonlName);
      if (!existsSync(jsonlPath)) continue;

      const relativeJsonlPath = displayProjectPath(projectRoot, jsonlPath, taskDir);
      const manifest = readJsonlLines(jsonlPath, relativeJsonlPath);
      if (manifest.omitted) {
         appendCandidate(`## ${jsonlName}\n\n${manifest.omitted}`);
         continue;
      }

      const fileChunks: string[] = [];
      let sectionHeaderEmitted = false;
      for (const line of manifest.lines) {
         const trimmed = line.trim();
         if (!trimmed) continue;
         try {
            const row = JSON.parse(trimmed) as Record<string, unknown>;
            const file = typeof row.file === "string" ? row.file.trim() : "";
            if (!file) continue;
            const targetPath = resolveProjectFile(projectRoot, file, trustedRoots);
            if (!targetPath) continue;
            if (includedPaths.has(targetPath)) continue;
            includedPaths.add(targetPath);
            const materialized = materialize(targetPath, file, typeof row.reason === "string" ? row.reason : "-", limits.max_file_bytes, "file");
            const sectionPrefix = sectionHeaderEmitted
               ? "\n\n---\n\n"
               : `${parts.length > 0 ? "\n\n" : ""}## ${jsonlName}\n\n`;
            const output = budget.emitCandidate(
               materialized.block === null ? null : `${sectionPrefix}${materialized.block}`,
               `${sectionPrefix}${materialized.notice}`,
            );
            if (output !== null) {
               fileChunks.push(output);
               sectionHeaderEmitted = true;
            }
         } catch {
            // 种子行与格式错误的行不是致命错误
         }
      }

      if (fileChunks.length > 0) parts.push(fileChunks.join(""));
   }

   if (parts.length === 0) return "";
   const context = `${prefix}${parts.join("")}${suffix}`;
   if (limits.max_total_bytes <= 0 || Buffer.byteLength(context, "utf-8") <= limits.max_total_bytes) return context;
   const suffixBytes = Buffer.byteLength(suffix, "utf-8");
   const bodyLimit = Math.max(0, limits.max_total_bytes - suffixBytes);
   const body = truncateUtf8(Buffer.from(`${prefix}${parts.join("")}`, "utf-8"), bodyLimit).toString("utf-8");
   return `${body}${suffix}`;
}

// ---------------------------------------------------------------------------
// 提示注入配置（逃生口）
// ---------------------------------------------------------------------------

// 与 sunshine hook workflow-state 的 DEFAULT_PROMPT_INJECTION_SKIP_KEYWORD 一致：
// 跳过关键词默认为 "no-sunshine"；显式设为 "" 则完全禁用
// 这个逃生口。
const DEFAULT_PROMPT_INJECTION_SKIP_KEYWORD = "no-sunshine";

// 按 PyYAML 兼容方式解析会被解析为非字符串的标量：null
// （空、~、null 变体）、bool（YAML 1.1 集合）与数字。带引号的标量
// 不会走到这项检查，始终是字符串。
function isYamlNonStringScalar(raw: string): boolean {
   if (raw === "" || raw === "~" || /^(?:null|Null|NULL)$/.test(raw)) return true;
   if (/^(?:true|True|TRUE|false|False|FALSE|yes|Yes|YES|no|No|NO|on|On|ON|off|Off|OFF)$/.test(raw)) return true;
   // PyYAML 整数解析：二进制/八进制/十进制/十六进制；前导零的十进制不是整数。
   if (/^[-+]?(?:0[bB][01_]+|0[0-7_]+|0[xX][0-9a-fA-F_]+|[1-9][\d_]*|0)$/.test(raw)) return true;
   // PyYAML 浮点解析：要求有点与带符号的指数（"1.5e+3"，
   // 而不是 "1.5e3"——后者在 PyYAML 里仍是字符串）。
   return /^[-+]?(?:\d[\d_]*\.[\d_]*|\.[\d_]+)(?:[eE][-+]\d+)?$/.test(raw) ||
      /^[-+]?\.(?:inf|Inf|INF)$/.test(raw) ||
      /^\.(?:nan|NaN|NAN)$/.test(raw);
}

function readPromptInjectionSkipKeyword(projectRoot: string): string {
   let config = "";
   try { config = readFileSync(join(projectRoot, ".sunshine", "config.yaml"), "utf-8"); } catch { return DEFAULT_PROMPT_INJECTION_SKIP_KEYWORD; }

   let inSection = false;
   let sectionIndent = -1;
   for (const rawLine of config.split(/\r?\n/)) {
      const trimmed = rawLine.trim();
      if (!inSection) {
         if (/^prompt_injection\s*:\s*(#.*)?$/.test(trimmed)) {
            inSection = true;
            sectionIndent = rawLine.length - rawLine.trimStart().length;
         }
         continue;
      }
      if (!trimmed || trimmed.startsWith("#")) continue;
      const indent = rawLine.length - rawLine.trimStart().length;
      if (indent <= sectionIndent) break;
      const match = trimmed.match(/^skip_keyword\s*:\s*(.*)$/);
      if (!match) continue;
      const rawValue = stripInlineComment(match[1]!).trim();
      const unquoted = unquoteYaml(rawValue);
      // 保留 YAML 标量类型，与 skip 关键词解析的“原始值须为字符串”检查一致：
      // 裸非字符串标量（bool/null/数字，包括空值）回退到默认值，而
      // 带引号的标量——包括显式的 ——保持字符串。

      if (unquoted === rawValue && isYamlNonStringScalar(rawValue)) {
         return DEFAULT_PROMPT_INJECTION_SKIP_KEYWORD;
      }
      return unquoted.trim();
   }
   return DEFAULT_PROMPT_INJECTION_SKIP_KEYWORD;
}

// 与 sunshine hook workflow-state 的 prompt_has_skip_keyword() 一致：连字符算
// 单词字符，所以 "no-sunshinex" / "xno-sunshine" / "foo-no-sunshine" 不
// 匹配，但标点/空白边界会匹配。空关键词永不匹配。
function shouldSkipWorkflowState(
   userInput: string,
   skipKeyword: string,
): boolean {
   if (!skipKeyword) return false;
   const escapedKeyword = skipKeyword.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
   const pattern = new RegExp(`(?<![\\w-])${escapedKeyword}(?![\\w-])`, "i");
   return pattern.test(userInput);
}

// ---------------------------------------------------------------------------
// 每轮缓存——避免在单个事件级联内重复解析工作流状态
// （input、before_agent_start 与 context 相继触发）
// ---------------------------------------------------------------------------

const SESSION_OVERVIEW_TEXT =
   "Sunshine 工作流系统已激活。请按工作流状态的指示使用 skills 与 agent。";

class TurnContextCache {
   private key: string | null = null;
   private timestamp = 0;
   private workflowMsg = "";
   private skipThisTurn = false;
   private static readonly TTL_MS = 1500;

   // 每用户轮次调用一次（input 事件），带入本轮的跳过决定；
   // 让级联里的每个读取方（before_agent_start、context）解析到同一轮次状态。

   beginTurn(skipThisTurn: boolean): void {
      this.skipThisTurn = skipThisTurn;
      this.key = null;
   }

   get(projectRoot: string, contextKey: string | null): { workflowMsg: string } {
      const now = Date.now();
      const cacheKey = `${projectRoot}:${contextKey ?? ""}`;
      if (
         this.key === cacheKey &&
         now - this.timestamp < TurnContextCache.TTL_MS
      ) {
         return { workflowMsg: this.workflowMsg };
      }

      const { status } = resolveActiveTaskStatus(projectRoot, contextKey);

      const workflowPath = join(projectRoot, ".sunshine", "workflow.md");
      let workflowMd = "";
      try { workflowMd = readFileSync(workflowPath, "utf-8"); } catch { }

      let workflowBody = "";
      if (workflowMd) {
         const blocks = parseWorkflowStateBlocks(workflowMd);
         const activeBlock = blocks.find((b) => b.status === status);
         if (activeBlock) {
            workflowBody = `[workflow-state:${activeBlock.status}]\n${activeBlock.content}\n[/workflow-state:${activeBlock.status}]`;
         }
      }
      if (!workflowBody) {
         workflowBody = "请查看 workflow.md 了解当前步骤。";
      }

      // 本轮出现跳过关键词时，跳过本轮的工作流状态注入
      this.workflowMsg = this.skipThisTurn
         ? ""
         : `<workflow-state>\n${workflowBody}\n</workflow-state>\n\n<session-overview>\n${SESSION_OVERVIEW_TEXT}\n</session-overview>`;

      this.key = cacheKey;
      this.timestamp = now;
      return { workflowMsg: this.workflowMsg };
   }
}

// ---------------------------------------------------------------------------
// 工作流状态标签解析
// ---------------------------------------------------------------------------

const WORKFLOW_STATE_RE =
   /\[workflow-state:([A-Za-z0-9_-]+)\]\s*\n([\s\S]*?)\n\s*\[\/workflow-state:\1\]/g;

interface WorkflowStateBlock {
   status: string;
   content: string;
}

function parseWorkflowStateBlocks(markdown: string): WorkflowStateBlock[] {
   const blocks: WorkflowStateBlock[] = [];
   for (const match of markdown.matchAll(WORKFLOW_STATE_RE)) {
      blocks.push({
         status: match[1],
         content: match[2].trim(),
      });
   }
   return blocks;
}

// ---------------------------------------------------------------------------
// 子代理检测
// ---------------------------------------------------------------------------

const SUNSHINE_AGENTS = new Set(["sunshine-implement", "sunshine-check", "sunshine-research"]);

function detectAgentType(): AgentType {
   const blocked = process.env.PI_BLOCKED_AGENT;
   if (blocked && SUNSHINE_AGENTS.has(blocked)) {
      return blocked as AgentType;
   }
   return null;
}

interface TaskContextCache {
   projectRoot: string;
   taskDir: string;
   agentType: AgentType;
   signature: string;
   content: string;
}

// ---------------------------------------------------------------------------
// 扩展入口
// ---------------------------------------------------------------------------

export default function(pi: ExtensionAPI): void {
   let projectRoot: string | null = null;
   const turnCache = new TurnContextCache();
   const agentType = detectAgentType();
   const isSubAgent = agentType !== null;
   let taskContextCache: TaskContextCache | null = null;

   // 跟踪压缩边界——自上次注入后没有发生压缩时，上下文处理跳过扫描。

   let lastCompactionTs = 0;
   let lastInjectionTs = 0;

   const rememberContextKey = (ctx?: { sessionManager?: { getSessionId?: () => string | undefined; getSessionFile?: () => string | undefined } }): string | null => {
      const key = deriveContextKey(ctx);
      if (!key) return null;
      return key;
   };

   const getTaskContext = (taskDir: string, root: string): string => {
      const signature = taskContextSignature(root, taskDir, agentType);
      if (taskContextCache?.projectRoot === root && taskContextCache.taskDir === taskDir && taskContextCache.agentType === agentType && taskContextCache.signature === signature) {
         return taskContextCache.content;
      }
      const content = buildTaskContext(root, taskDir, agentType);
      taskContextCache = { projectRoot: root, taskDir, agentType, signature, content };
      return content;
   };

   pi.on("session_start", async (_event, ctx) => {
      projectRoot = findProjectRoot(ctx.cwd);
      const contextKey = rememberContextKey(ctx);

      if (!projectRoot) return;

      if (isSubAgent) {
         // 子代理：只注入一次精确的任务上下文
         const { taskDir } = resolveActiveTaskStatus(projectRoot, contextKey);
         if (taskDir) {
            const taskContext = getTaskContext(taskDir, projectRoot);
            if (taskContext) {
               await pi.sendMessage({
                  customType: "sunshine-task-context",
                  content: taskContext,
                  display: false,
               });
            }
         }
      } else {
         // 主会话：注入会话上下文（全局映射）+ 任务上下文
         const sessionContext = buildSessionContext(projectRoot, contextKey);
         if (sessionContext) {
            await pi.sendMessage({
               customType: "sunshine-session-context",
               content: sessionContext,
               display: false,
            });
         }

         const { taskDir } = resolveActiveTaskStatus(projectRoot, contextKey);
         if (taskDir) {
            const taskContext = getTaskContext(taskDir, projectRoot);
            if (taskContext) {
               await pi.sendMessage({
                  customType: "sunshine-task-context",
                  content: taskContext,
                  display: false,
               });
            }
         }

         ctx.ui.notify("Sunshine 工作流系统可用", "info");
      }
   });

   pi.on("session_before_compact", async () => {
      lastCompactionTs = Date.now();
   });

   pi.on("before_agent_start", async (_event, ctx) => {
      if (!projectRoot) {
         projectRoot = findProjectRoot(ctx.cwd);
      }
      if (!projectRoot) return;
      const contextKey = rememberContextKey(ctx);

      // 持久注入：本轮的工作流状态
      const cached = turnCache.get(projectRoot, contextKey);
      lastInjectionTs = Date.now();

      // 跳过轮次：什么都不注入（逃生口）
      if (!cached.workflowMsg) return;

      return {
         message: {
            customType: "sunshine-workflow-state",
            content: cached.workflowMsg,
            display: false,
         },
      };
   });

   // context 在每次 LLM API 调用前触发（包括工具使用延续
   // 与压缩后的 agent.continue() 路径）。当 before_agent_start 的持久化消息
   // 被压缩删除时，它是安全网。
   pi.on("context", async (event, ctx) => {
      if (!projectRoot) return;
      const contextKey = rememberContextKey(ctx);

      const messages = event.messages as { role?: string; customType?: string; content?: string }[];
      const { taskDir } = resolveActiveTaskStatus(projectRoot, contextKey);
      const currentTaskContext = taskDir ? getTaskContext(taskDir, projectRoot) : "";
      const taskContextIndexes = messages
         .map((message, index) => message.customType === "sunshine-task-context" ? index : -1)
         .filter((index) => index >= 0);
      const existingTaskContext = taskContextIndexes.length > 0
         ? messages[taskContextIndexes[0]]?.content ?? ""
         : "";
      const taskContextChanged = existingTaskContext !== currentTaskContext || taskContextIndexes.length > 1;
      let projectedMessages = messages;
      if (taskContextChanged) {
         const replacement = currentTaskContext
            ? { role: "custom" as const, customType: "sunshine-task-context", content: currentTaskContext, display: false, timestamp: Date.now() }
            : null;
         let replaced = false;
         projectedMessages = messages.flatMap((message) => {
            if (message.customType !== "sunshine-task-context") return [message];
            if (replaced || !replacement) return [];
            replaced = true;
            return [replacement];
         });
         if (replacement && !replaced) projectedMessages.push(replacement);
      }

      // 在快速路径之前先解析轮次状态：跳过轮次仍须执行面包屑清理，
      // 即使其他内容都没变。
      const cached = turnCache.get(projectRoot, contextKey);
      const skipping = !cached.workflowMsg;

      // 快速路径：任务没变、无压缩、不跳过——所有持久化上下文都是最新的。
      if (!taskContextChanged && !skipping && lastInjectionTs > lastCompactionTs) return;

      if (skipping) {
         // 跳过轮次（逃生口）：丢弃之前轮次留下的所有持久化面包屑，
         // 让跳过真正生效。
         const withoutBreadcrumb = projectedMessages.filter(
            (message) => !(message.role === "custom" && message.customType === "sunshine-workflow-state"),
         );
         if (withoutBreadcrumb.length === projectedMessages.length && !taskContextChanged) return;
         lastInjectionTs = Date.now();
         return { messages: withoutBreadcrumb };
      }

      // 压缩后：注入前反向扫描确认不存在
      for (let i = projectedMessages.length - 1; i >= 0; i--) {
         if (projectedMessages[i].role === "custom" && projectedMessages[i].customType === "sunshine-workflow-state") {
            lastInjectionTs = Date.now();
            return taskContextChanged ? { messages: projectedMessages } : undefined;
         }
      }

      lastInjectionTs = Date.now();
      return {
         messages: [
            ...projectedMessages,
            {
               role: "custom" as const,
               customType: "sunshine-workflow-state",
               content: cached.workflowMsg,
               display: false,
               timestamp: Date.now(),
            },
         ],
      };
   });

   // OMP 把 Bash event.input 直通到工具执行参数，所以通过与 shell 无关的
   // env 字段注入会话键。显式的单次调用值优先于派生的键。

   pi.on("tool_call", (event, ctx) => {
      if (event.toolName !== "bash") return;
      const contextKey = rememberContextKey(ctx);
      if (!contextKey) return;
      const input = event.input as { env?: Record<string, string> };
      input.env = {
         SUNSHINE_CONTEXT_ID: contextKey,
         ...input.env,
      };
   });

   pi.on("input", async (event, ctx) => {
      if (!projectRoot) {
         projectRoot = findProjectRoot(ctx.cwd);
      }
      // 如果 session_start 错过了，在首次 input 时解析 projectRoot
      if (!projectRoot) return;
      const contextKey = rememberContextKey(ctx);

      // 检查本轮是否应跳过工作流状态注入
      const skipKeyword = readPromptInjectionSkipKeyword(projectRoot);
      const skipThisTurn = shouldSkipWorkflowState(event.text ?? "", skipKeyword);

      // 记录本轮的跳过决定并预热缓存，供
      // before_agent_start 与 context 使用
      turnCache.beginTurn(skipThisTurn);
      turnCache.get(projectRoot, contextKey);
   });
}
