package openai

import "testing"

func TestEnsureCodexUserAgentTrailer(t *testing.T) {
	cases := []struct{ in, og, ver, want string }{
		{"codex_cli_rs/0.153.3 (Ubuntu 22.4.0; x86_64) xterm-256color", "codex_cli_rs", "0.153.3",
			"codex_cli_rs/0.153.3 (Ubuntu 22.4.0; x86_64) xterm-256color (codex_cli_rs; 0.153.3)"},
		// TUI 形态：尾段是版本号 → 同步
		// TUI：尾段版本与首段一致 → 同步尾段（首段由 SetCodexUserAgentVersion 处理）
		{"codex-tui/0.155.1 (Mac OS 27.0.0; arm64) xterm-256color (codex-tui; 0.155.1)", "codex-tui", "0.153.3",
			"codex-tui/0.155.1 (Mac OS 27.0.0; arm64) xterm-256color (codex-tui; 0.153.3)"},
		// Desktop 形态：尾段是构建号 → 保留
		// 构建号尾段：本函数不动（首段版本由 SetCodexUserAgentVersion 负责）
		{"Codex Desktop/0.155.0-alpha.9.2 (Mac OS 27.0.0; arm64) unknown (Codex Desktop; 26.915.31945)", "Codex Desktop", "0.153.3",
			"Codex Desktop/0.155.0-alpha.9.2 (Mac OS 27.0.0; arm64) unknown (Codex Desktop; 26.915.31945)"},
		// 已一致 → 不动
		{"codex_exec/0.153.3 (x) unknown (codex_exec; 0.153.3)", "codex_exec", "0.153.3",
			"codex_exec/0.153.3 (x) unknown (codex_exec; 0.153.3)"},
	}
	for _, c := range cases {
		if got := EnsureCodexUserAgentTrailer(c.in, c.og, c.ver); got != c.want {
			t.Errorf("in=%q\n got=%q\nwant=%q", c.in, got, c.want)
		}
	}
}
