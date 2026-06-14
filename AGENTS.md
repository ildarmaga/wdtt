# WDTT — Agent instructions

Open this repo as the workspace root (`File → Open Folder → wdtt` or `wdtt.code-workspace`), not parent `/root`, so project hooks and `.cursor/mcp.json` apply.

## Rules (in git)

See `.cursor/rules/wdtt/`:

- **project.mdc** — layout, build, deploy
- **paneldb.mdc** — SQLite / `pkg/paneldb`
- **panel-api.mdc** — legacy vs native panel API

Local ECC (skills, extra rules, hooks) stays under `.cursor/` but is gitignored except `rules/wdtt/`.

## Quick commands

```bash
./build.sh amd64 all
systemctl is-active wdtt wdtt-panel
curl -s http://127.0.0.1:2861/health
```

## Docs

- `docs/SERVER.md` — server
- `docs/API.md` — panel REST
