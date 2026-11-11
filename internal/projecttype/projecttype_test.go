package projecttype

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFixture 在 dir 下按相对路径写一个文件（自动建父目录）。
func writeFixture(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)
		want  Type
	}{
		// 前端
		{
			name:  "前端构建配置 vite.config.ts",
			setup: func(t *testing.T, dir string) { writeFixture(t, dir, "vite.config.ts", "") },
			want:  Frontend,
		},
		{
			name:  "前端构建配置 next.config.mjs",
			setup: func(t *testing.T, dir string) { writeFixture(t, dir, "next.config.mjs", "") },
			want:  Frontend,
		},
		{
			name: "前端入口 src/App.tsx",
			setup: func(t *testing.T, dir string) {
				writeFixture(t, dir, "src/App.tsx", "export default function App() {}")
			},
			want: Frontend,
		},
		{
			name:  "前端入口 pages/index.jsx",
			setup: func(t *testing.T, dir string) { writeFixture(t, dir, "pages/index.jsx", "") },
			want:  Frontend,
		},
		{
			name: "前端依赖 react",
			setup: func(t *testing.T, dir string) {
				writeFixture(t, dir, "package.json", `{"dependencies": {"react": "^18.0.0"}}`)
			},
			want: Frontend,
		},
		{
			name: "前端依赖在 devDependencies 里也算",
			setup: func(t *testing.T, dir string) {
				writeFixture(t, dir, "package.json", `{"devDependencies": {"vue": "^3.0.0"}}`)
			},
			want: Frontend,
		},
		// 后端
		{
			name:  "只有 go.mod 无 package.json",
			setup: func(t *testing.T, dir string) { writeFixture(t, dir, "go.mod", "module example.com/x\n\ngo 1.27\n") },
			want:  Backend,
		},
		{
			name: "只有 express 依赖的 package.json 不是全栈",
			setup: func(t *testing.T, dir string) {
				// 钉住与上游的偏离：package.json 存在本身不算前端信号。
				writeFixture(t, dir, "package.json", `{"dependencies": {"express": "^4.0.0"}}`)
			},
			want: Backend,
		},
		{
			name:  "后端 Gemfile",
			setup: func(t *testing.T, dir string) { writeFixture(t, dir, "Gemfile", "source 'https://rubygems.org'\n") },
			want:  Backend,
		},
		{
			name:  "后端 requirements.txt",
			setup: func(t *testing.T, dir string) { writeFixture(t, dir, "requirements.txt", "flask\n") },
			want:  Backend,
		},
		{
			name:  "后端 Cargo.toml",
			setup: func(t *testing.T, dir string) { writeFixture(t, dir, "Cargo.toml", "[package]\nname = \"x\"\n") },
			want:  Backend,
		},
		{
			name:  "后端 .NET 项目文件",
			setup: func(t *testing.T, dir string) { writeFixture(t, dir, "app.csproj", "<Project />") },
			want:  Backend,
		},
		{
			name:  "后端入口 src/index.ts",
			setup: func(t *testing.T, dir string) { writeFixture(t, dir, "src/index.ts", "") },
			want:  Backend,
		},
		{
			name:  "后端 server.js",
			setup: func(t *testing.T, dir string) { writeFixture(t, dir, "server.js", "") },
			want:  Backend,
		},
		// 全栈
		{
			name: "react 加 express 依赖",
			setup: func(t *testing.T, dir string) {
				writeFixture(t, dir, "package.json", `{"dependencies": {"react": "^18.0.0", "express": "^4.0.0"}}`)
			},
			want: Fullstack,
		},
		{
			name: "vite.config.ts 加 go.mod",
			setup: func(t *testing.T, dir string) {
				writeFixture(t, dir, "vite.config.ts", "")
				writeFixture(t, dir, "go.mod", "module example.com/x\n")
			},
			want: Fullstack,
		},
		{
			name: "前端文件加后端依赖",
			setup: func(t *testing.T, dir string) {
				writeFixture(t, dir, "src/App.vue", "")
				writeFixture(t, dir, "package.json", `{"dependencies": {"fastify": "^4.0.0"}}`)
			},
			want: Fullstack,
		},
		// 未知
		{
			name:  "空目录",
			setup: func(t *testing.T, dir string) {},
			want:  Unknown,
		},
		{
			name: "package.json 损坏不是合法 JSON",
			setup: func(t *testing.T, dir string) {
				writeFixture(t, dir, "package.json", `{"dependencies": {`)
			},
			want: Unknown,
		},
		{
			name: "package.json 只有无关依赖 lodash",
			setup: func(t *testing.T, dir string) {
				writeFixture(t, dir, "package.json", `{"dependencies": {"lodash": "^4.0.0"}}`)
			},
			want: Unknown,
		},
		{
			name:  "不存在的目录",
			setup: func(t *testing.T, dir string) {},
			want:  Unknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.name == "不存在的目录" {
				dir = filepath.Join(dir, "不存在的子目录")
			} else {
				tt.setup(t, dir)
			}
			if got := Detect(dir); got != tt.want {
				t.Errorf("Detect() = %v(%s)， want %v(%s)", got, got.String(), tt.want, tt.want.String())
			}
		})
	}
}

func TestTypeStringAndDisplayName(t *testing.T) {
	tests := []struct {
		typ     Type
		str     string
		display string
	}{
		{Unknown, "unknown", "未知"},
		{Frontend, "frontend", "前端"},
		{Backend, "backend", "后端"},
		{Fullstack, "fullstack", "全栈"},
		{Design, "design", "产品设计"},
	}
	for _, tt := range tests {
		if got := tt.typ.String(); got != tt.str {
			t.Errorf("Type(%d).String() = %q，want %q", int(tt.typ), got, tt.str)
		}
		if got := tt.typ.DisplayName(); got != tt.display {
			t.Errorf("Type(%d).DisplayName() = %q，want %q", int(tt.typ), got, tt.display)
		}
	}
	// 零值就是 Unknown。
	var zero Type
	if zero != Unknown || zero.String() != "unknown" || zero.DisplayName() != "未知" {
		t.Errorf("零值应为 Unknown，got %v", zero)
	}
}

func TestParse(t *testing.T) {
	valid := []struct {
		input string
		want  Type
	}{
		{"前端", Frontend},
		{"frontend", Frontend},
		{"后端", Backend},
		{"backend", Backend},
		{"全栈", Fullstack},
		{"前后端全栈", Fullstack},
		{"fullstack", Fullstack},
		{"产品设计", Design},
		{"design", Design},
	}
	for _, tt := range valid {
		t.Run("合法值 "+tt.input, func(t *testing.T) {
			got, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse(%q) 报错：%v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("Parse(%q) = %v，want %v", tt.input, got, tt.want)
			}
		})
	}

	invalid := []string{"unknown", "", "web", "Frontend", "BACKEND", "全端", "前后端", "mono", " 前端", "frontend "}
	for _, input := range invalid {
		t.Run("非法值 "+input, func(t *testing.T) {
			got, err := Parse(input)
			if err == nil {
				t.Fatalf("Parse(%q) 应报错，got %v", input, got)
			}
			want := "无效的项目类型：" + input + "（合法值：前端、后端、全栈、产品设计）"
			if err.Error() != want {
				t.Errorf("Parse(%q) 错误文案 = %q，want %q", input, err.Error(), want)
			}
			if got != Unknown {
				t.Errorf("Parse(%q) 出错时应返回 Unknown，got %v", input, got)
			}
		})
	}
}
