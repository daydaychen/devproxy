# PKG/ PACKAGE KNOWLEDGE BASE

**Generated:** 2026-02-01
**Package:** 4 subdirs, 9 files, 648 lines

## OVERVIEW
config loads YAML, proxy handles MITM, process launches target commands, util provides FD hijacking

## STRUCTURE
```
pkg/
├── config/config.go           # YAML unmarshaling (RuleConfig, Config structs)
├── proxy/
│   ├── server.go              # MITM server with goproxy, dynamic ShouldMITM
│   ├── matcher.go             # URLMatcher interface, RegexMatcher/StringMatcher
│   └── rewriter.go            # HeaderRewriter struct, Set() on req.Header
├── process/launcher.go        # PTY detection, SIGWINCH handling, env injection
└── util/
    ├── io.go                  # syscall.Dup2 FD hijacking, returns orig streams
    ├── ansi.go                # AnsiStripper/CrLfFixer as io.Writer wrappers
    ├── version.go             # Variable for -X ldflags injection
    └── port.go                # Random port allocation via net.Listen
```

## WHERE TO LOOK
| Task | File | Notes |
|------|------|-------|
| Add matcher type | proxy/matcher.go | Implement URLMatcher interface |
| Modify MITM logic | proxy/server.go:185 | ShouldMITM function |
| PTY mode behavior | process/launcher.go:88 | startWithPty, SIGWINCH channel |
| ANSI handling | util/ansi.go:24 | AnsiStripper.Write() wrapper |
| FD redirection | util/io.go:10 | syscall.Dup2 level |

## CONVENTIONS
- **Interface-driven matching**: URLMatcher with concrete RegexMatcher/StringMatcher
- **Rule grouping**: ProxyRule combines Matchers + Rewriters in single unit
- **Default rule pattern**: ProxyServer.defaultRule stores global matchers/rewriters
- **PTY detection**: ProcessLauncher checks term.IsTerminal() before PTY spawn
- **Window resize**: SIGWINCH signal -> pty.InheritSize() in goroutine
- **Writer wrappers**: AnsiStripper/CrLfFixer implement io.Writer
- **FD hijacking**: Physical Dup2 + Go os.Stdout sync in HijackStandardStreams
- **Version injection**: util.Version variable overwritten by Makefile -X ldflags

## ANTI-PATTERNS
- **Ignored io.Copy errors**: launcher.go:113,118 (silent failure)
- **Conservative MITM**: ShouldMITM defaults to true for regex/path patterns
- **AnsiStripper length hack**: Returns len(p) despite writing stripped bytes
- **CrLfFixer allocation**: Reallocates buffer for every Write() call
- **No internal/**: All pkg/ packages are public API
