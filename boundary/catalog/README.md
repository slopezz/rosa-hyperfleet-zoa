# Boundary offline catalogs (Trusted Actions + break-glass)

Neither file is stored in git. Each boundary task start runs:

```bash
zoa actions --offline -o markdown > /home/sre/.claude/ZOA_ACTIONS.md
zoa breakglass scopes --offline -o markdown --target-type "${ZOA_TARGET_TYPE:-mc}" > /home/sre/.claude/ZOA_BREAKGLASS_SCOPES.md
```

Target type (`rc` or `mc`) comes from **`ZOA_TARGET_TYPE`** (set by Access from target metadata).
