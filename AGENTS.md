# PROJECT KNOWLEDGE BASE

**Generated:** 2026-03-05
**Commit:** 6b49de1
**Branch:** main

## OVERVIEW
Go-based MITM proxy for development/testing that intercepts and modifies HTTP/HTTPS traffic. Bypasses TLS verification intentionally - NOT for production use.

## STRUCTURE
```
devproxy/
├── main.go                  # Entry point (delegates to cmd/root.go)
├── cmd/                     # CLI framework (Cobra)
│   ├── root.go             # Main command logic
│   └── AGENTS.md           # CLI-specific notes
├── pkg/                    # Library code (public)
│   ├── config/            # YAML config loading
│   ├── proxy/             # MITM proxy server + URL matching + plugins
│   │   ├── plugin.go              # Plugin interface & registry
│   │   ├── plugin_codex.go        # Codex API transformation plugin
│   │   ├── plugin_openai_responses.go  # OpenAI Responses API support
│   │   ├── matcher.go            # URL matching logic
│   │   ├── matcher_test.go       # Unit tests
│   │   ├── server.go             # MITM proxy core
│   │   └── rewriter.go           # Header rewriting
│   ├── process/           # Process launching
│   │   ├── launcher.go    # Process wrapper
│   │   └── certs.go       # Certificate management
│   └── util/              # Terminal I/O, FD hijacking, ANSI stripping
│       └── AGENTS.md      # Package-specific notes
├── examples/              # Sample configurations
├── .sisyphus/             # Rust rewrite planning (non-standard)
└── Makefile              # Build automation (build, build-opt, release, clean, test, lint)
```

**New in v0.2.0+:** Plugin system with Codex and OpenAI Responses API support

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Add new CLI flag | `cmd/root.go` ~line 28 | Uses Cobra CLI framework |
| Modify URL matching | `pkg/proxy/matcher.go` | Regex/String matchers + URL normalization |
| Add header rewriting | `pkg/proxy/rewriter.go` | HeaderRewriter struct |
| Process launching | `pkg/process/launcher.go` | Direct process wrapping (no PTY) |
| Configuration | `pkg/config/config.go` | YAML loading + RuleConfig struct |
| Terminal I/O hijacking | `pkg/util/io.go` | FD-level stream redirection |
| Proxy server | `pkg/proxy/server.go` | MITM logic with goproxy |
| Add new plugin | `pkg/proxy/plugin.go` | Plugin interface & registry |
| Plugin: Codex API | `pkg/proxy/plugin_codex.go` | Model field substitution |
| Plugin: OpenAI Responses | `pkg/proxy/plugin_openai_responses.go` | SSE/tool_call handling |
| Certificate management | `pkg/process/certs.go` | Certificate installation |

## CODE MAP
| Symbol | Type | Location | Refs | Role |
|--------|------|----------|------|------|
| `Execute` | func | `cmd/root.go:24` | 1 | Cobra entry point |
| `NewProxyServer` | func | `pkg/proxy/server.go:34` | 1 | MITM proxy creation |
| `ProcessLauncher` | struct | `pkg/process/launcher.go:16` | 3 | Interactive process handler |
| `ShouldMITM` | func | `pkg/proxy/server.go:185` | 1 | Decides when to decrypt HTTPS |
| `HijackStandardStreams` | func | `pkg/util/io.go:10` | 1 | FD-level stdout/stderr redirection |
| `Plugin` | interface | `pkg/proxy/plugin.go:10` | 3 | Plugin contract |
| `RegisterPlugin` | func | `pkg/proxy/plugin.go:28` | 1 | Register plugin to proxy |
| `CodexPlugin` | struct | `pkg/proxy/plugin_codex.go:18` | 1 | Codex API transformer |
| `OpenAIResponsesPlugin` | struct | `pkg/proxy/plugin_openai_responses.go:28` | 1 | OpenAI Responses handler |
| `EnsureCert` | func | `pkg/process/certs.go:12` | 1 | Certificate installation |

## CONVENTIONS
- **Chinese comments**: Code comments mixed Chinese/English (e.g., Makefile line 11)
- **Tests exist**: `matcher_test.go`, `plugin_codex_test.go`, `plugin_openai_responses_test.go`
- **Test location**: Tests live alongside source files
- **Go version**: Requires Go 1.23+ (check `go.mod`)
- **FD hijacking**: Uses syscall.Dup2 for physical stream redirection
- **No internal/**: All packages in pkg/ directory (implies public)

## ANTI-PATTERNS (THIS PROJECT)
1. **TLS bypass**: 8+ env vars disable certificate verification (pkg/process/launcher.go:55-62)
2. **Production warning**: README explicitly warns "仅适用于开发和测试环境"
3. **Fatal errors**: log.Fatalf used throughout cmd/root.go (lines 58, 198, 219, etc.)
4. **Error suppression**: io.Copy errors ignored (pkg/process/launcher.go:113, 118)
5. **Hardcoded paths**: Makefile installs to `$(HOME)/.local/bin/` without override
6. **Orphaned reference**: Makefile clean removes `$(BINARY_NAME)-std` never created

## UNIQUE FEATURES
- **Plugin system**: Extensible request/response transformation (v0.2.0+)
- **Interactive app support**: Raw terminal mode, ANSI stripping (pkg/util/ansi.go)
- **Multi-level config**: 4 config sources merged (global/dir/explicit/CLI)
- **Log isolation**: Proxy worker runs in separate process to prevent terminal pollution
- **Dynamic MITM**: `ShouldMITM` function decides per-host whether to decrypt
- **Version injection**: `-X devproxy/pkg/util.Version=` via Makefile LDFLAGS
- **URL normalization**: Default ports (443/80) now handled correctly
- **Certificate trust**: `pkg/util/trust_cert_mac.sh` for macOS cert installation
- **SSE streaming**: Proper handling of Server-Sent Events with compression

## COMMANDS
```bash
make build          # Standard build
make build-opt      # Optimized (-s -w flags)
make release        # Install to ~/.local/bin/
make clean          # Remove binaries
make test           # Run unit tests
make lint           # Run golangci-lint
make check          # Run tests + lint

# Development
./test_fix.sh       # Test interactive apps
./test_interactive.sh
```

## NOTES
- **Security**: TLS verification disabled - MITM proxy intentionally insecure (development only)
- **Process model**: Parent process proxies, child runs target command
- **Interactive apps**: Direct process wrapping (no PTY since commit 829ba6c)
- **Architecture**: Simplified to single-process with worker goroutines
- **Rust plans**: `.sisyphus/rust-rewrite-plan.md` exists (427 lines)
- **No CI/CD**: `.github/workflows/` missing despite extensive .github templates
