# t4mem Codex Plugin

This plugin packages `t4mem` for Codex as:

- a bundled `t4mem-agent-memory` skill
- a local MCP server definition
- a launcher that runs a released `t4memd` binary

## Install

The first launch auto-downloads the matching release binary into the plugin's
`bin/` directory. If you want to preinstall it yourself, run:

```bash
./plugins/t4mem/scripts/install_t4memd_release.sh
```

Optional environment overrides:

- `T4MEM_VERSION=v0.1.0` to pin a specific GitHub release tag
- `T4MEM_RELEASE_OWNER=...` to override the GitHub org/user
- `T4MEM_RELEASE_REPO=...` to override the repository name
- `T4MEMD_BIN=/custom/path/t4memd` to bypass the plugin-managed binary
- `T4MEM_ROOT=/custom/path/.t4mem` to override the memory root

## Runtime

The MCP config launches:

```bash
/bin/sh ./scripts/launch_t4memd.sh
```

If `bin/t4memd` is missing, the launcher first runs the installer, then it
resolves the repository root and executes:

```bash
<installed-binary> -root <repo>/.t4mem
```
