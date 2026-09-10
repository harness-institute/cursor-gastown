# Cursor Gas Town — fork lineage

This repository is **[Cursor Gas Town](https://github.com/harness-institute/cursor-gastown)**,
maintained by [Harness Institute](https://github.com/harness-institute). It tracks
current upstream **[Gas Town](https://github.com/gastownhall/gastown)** and is
optimized for the **Cursor Agent CLI** (`cursor-agent`).

| | Upstream | This fork |
|---|----------|-----------|
| Repository | [gastownhall/gastown](https://github.com/gastownhall/gastown) | [harness-institute/cursor-gastown](https://github.com/harness-institute/cursor-gastown) |
| Go module | `github.com/steveyegge/gastown` | `github.com/harness-institute/cursor-gastown` |
| npm package | `@gastown/gt` | `@harness-institute/cursor-gastown` |
| Default agent focus | Multi-agent (Claude, Copilot, Codex, …) | **Cursor-first** presets and docs |

Upstream owns the core architecture (Mayor, rigs, polecats, convoys, beads, refinery).
We rebase onto upstream periodically and keep only fork-specific branding, install
paths, and Cursor CLI tuning that upstream has not merged yet.

**Install (Go):**

```bash
go install github.com/harness-institute/cursor-gastown/cmd/gt@latest
```

**Prerequisites:** Go 1.26+, beads (`bd`) 1.2+, Cursor CLI (`cursor-agent`), git 2.25+, tmux 3.0+ (recommended).

See [README.md](README.md) for full usage and [docs/INSTALLING.md](docs/INSTALLING.md) for setup.
