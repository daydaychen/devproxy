# CMD PACKAGE KNOWLEDGE BASE

**Generated:** 2026-02-01
**Parent:** devproxy/AGENTS.md

## OVERVIEW
CLI entry point - multi-process architecture spawning proxy worker + target process

## STRUCTURE
```
cmd/
└── root.go (413 lines)
    ├── rootCmd              # Main command (args: <command> [args...])
    └── proxyWorkerCmd       # Hidden internal command: `__internal_proxy_worker`
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Config merge logic | `loadConfig()` (lines 24-98) | 4 sources merged in priority order |
| Proxy worker spawn | `run()` line 247 | `exec.Command(os.Args[0], "__internal_proxy_worker")` |
| Process forking flow | `run()` lines 245-271 | Parent → proxy worker → target process |
| Signal handling | `run()` lines 329-345 | SIGINT/SIGTERM cleanup |
| Flag parsing | `init()` lines 136-151 | 6 CLI flags, -v for version |

## CONVENTIONS
- **Config priority (low→high)**: global.yaml → dir-level (devproxy.yaml) → explicit (--config) → CLI flags
- **Config merge**: Single values override, lists accumulate (match, rules, overwrite)
- **Log isolation**: Proxy worker stdout/stderr redirected to file or discarded
- **Terminal handling**: Raw mode for interactive apps, CrLfFixer for log formatting
- **Memory optimization**: `debug.FreeOSMemory()` after component init, nil large arrays after fork

## ANTI-PATTERNS
- **Hard-coded executable**: `os.Args[0]` for proxy worker assumes binary location (line 247)
- **Blocking select**: `select {}` in proxy worker (line 205) - never exits gracefully
- **Mixed error handling**: Some errors log.Fatal (lines 58, 219, 265), others log.Printf + continue (line 61)

## DEPENDENCY FLOW
```
root.go
 ├─→ pkg/config: LoadConfig()        # YAML parsing
 ├─→ pkg/util:  GetRandomPort()      # Port allocation
 │            AnsiStripper           # Log formatting
 │            CrLfFixer               # Raw mode line endings
 ├─→ pkg/proxy: NewProxyServer()     # MITM server
 │            NewStringMatcher()      # URL matching
 │            NewHeaderRewriter()     # Header rewriting
 └─→ pkg/process: ProcessLauncher    # Target process launching
```

## INTERNAL COMMANDS
- `__internal_proxy_worker`: Hidden command (line 154), receives config via env vars (SMART_PROXY_*)
