# Boundary Trusted Actions catalog

`ZOA_ACTIONS.md` is **not stored in git**. Each boundary task start runs:

```bash
zoa actions --offline -o markdown > /home/sre/.claude/ZOA_ACTIONS.md
```

Target type (`rc` or `mc`) comes from **`ZOA_TARGET_TYPE`** (set by Access from target metadata).
