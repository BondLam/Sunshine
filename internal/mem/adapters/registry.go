package adapters

import "github.com/BondLam/Sunshine/internal/mem"

// All 按固定顺序返回 7 个会话适配器：
// claude、codex、devin、grok、opencode、pi、zcode。
func All() []mem.Adapter {
	return []mem.Adapter{
		ClaudeAdapter{},
		CodexAdapter{},
		DevinAdapter{},
		GrokAdapter{},
		OpencodeAdapter{},
		PiAdapter{},
		ZcodeAdapter{},
	}
}
