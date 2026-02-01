# PROJECT KNOWLEDGE BASE

**Generated:** 2026-02-01
**Commit:** 26fc4d9
**Branch:** main

## OVERVIEW
Go-based MITM proxy for development/testing that intercepts and modifies HTTP/HTTPS traffic. Bypasses TLS verification intentionally - NOT for production use.

## STRUCTURE
```
devproxy/
├── main.go                  # Entry point (delegates to cmd/root.go)
├── cmd/                     # CLI framework (Cobra)
│   └── root.go             # Main command logic (413 lines)
├── pkg/                    # Library code (public)
│   ├── config/            # YAML config loading
│   ├── proxy/             # MITM proxy server + URL matching
│   ├── process/           # Process launching + interactive app support
│   └── util/              # Terminal I/O, FD hijacking, ANSI stripping
├── examples/              # Sample configurations
├── .sisyphus/             # Rust rewrite planning (non-standard)
└── Makefile              # Build automation (build, build-opt, release, clean)
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Add new CLI flag | `cmd/root.go` ~line 28 | Uses Cobra CLI framework |
| Modify URL matching | `pkg/proxy/matcher.go` | Regex/String matchers |
| Add header rewriting | `pkg/proxy/rewriter.go` | HeaderRewriter struct |
| Process launching | `pkg/process/launcher.go` | PTY handling for interactive apps |
| Configuration | `pkg/config/config.go` | YAML loading + RuleConfig struct |
| Terminal I/O hijacking | `pkg/util/io.go` | FD-level stream redirection |
| Proxy server | `pkg/proxy/server.go` | MITM logic with goproxy |

## CODE MAP
| Symbol | Type | Location | Refs | Role |
|--------|------|----------|------|------|
| `Execute` | func | `cmd/root.go:24` | 1 | Cobra entry point |
| `NewProxyServer` | func | `pkg/proxy/server.go:34` | 1 | MITM proxy creation |
| `ProcessLauncher` | struct | `pkg/process/launcher.go:16` | 3 | Interactive process handler |
| `ShouldMITM` | func | `pkg/proxy/server.go:185` | 1 | Decides when to decrypt HTTPS |
| `HijackStandardStreams` | func | `pkg/util/io.go:10` | 1 | FD-level stdout/stderr redirection |

## CONVENTIONS
- **Chinese comments**: Code comments mixed Chinese/English (e.g., Makefile line 11)
- **No test files**: Zero `*_test.go` files in codebase (CRITICAL)
- **Multi-process architecture**: Proxy runs in separate worker process (`__internal_proxy_worker`)
- **FD hijacking**: Uses syscall.Dup2 for physical stream redirection
- **Blocking select**: `select {}` used in cmd/root.go line 205 (anti-pattern)
- **No internal/**: All packages in pkg/ directory (implies public)

## ANTI-PATTERNS (THIS PROJECT)
1. **TLS bypass**: 8+ env vars disable certificate verification (pkg/process/launcher.go:55-62)
2. **Production warning**: README explicitly warns "仅适用于开发和测试环境"
3. **Fatal errors**: log.Fatalf used throughout cmd/root.go (lines 58, 198, 219, etc.)
4. **Error suppression**: io.Copy errors ignored (pkg/process/launcher.go:113, 118)
5. **Hardcoded paths**: Makefile installs to `$(HOME)/.local/bin/` without override
6. **Orphaned reference**: Makefile clean removes `$(BINARY_NAME)-std` never created

## UNIQUE STYLES
- **Interactive app support**: Raw terminal mode, ANSI stripping (pkg/util/ansi.go)
- **Multi-level config**: 4 config sources merged (global/dir/explicit/CLI)
- **Log isolation**: Proxy worker runs in separate process to prevent terminal pollution
- **Dynamic MITM**: `ShouldMITM` function decides per-host whether to decrypt
- **Version injection**: `-X devproxy/pkg/util.Version=` via Makefile LDFLAGS

## COMMANDS
```bash
make build          # Standard build
make build-opt      # Optimized (-s -w flags)
make release        # Install to ~/.local/bin/
make clean          # Remove binaries

# Development
./test_fix.sh       # Test interactive apps
./test_interactive.sh
```

## NOTES
- **Security**: TLS verification disabled - MITM proxy intentionally insecure
- **Process model**: Parent process proxies, child runs target command
- **Interactive apps**: Uses PTY to support vim/bash/python interactive sessions
- **Architecture**: Recent rewrite to multi-process (see commit 8f6d095)
- **Rust plans**: `.sisyphus/rust-rewrite-plan.md` exists (427 lines)
- **No CI/CD**: `.github/workflows/` missing despite extensive .github templates
