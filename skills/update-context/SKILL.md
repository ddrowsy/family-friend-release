# update-context

Update the MCP file's manifest and directory entries whenever files change.

## Two Scenarios

### A. Session start — catch up with external changes

Run before starting any task:

```bash
bash bin/update-context.sh
```

Read output:
- `changed: ...` — files whose content changed
- `new: ...` — files not previously in manifest
- `deleted: ...` — files removed from project
- `new_manifest_json: ...` — full manifest JSON to replace the current one

Then:
1. Read each changed/new file
2. Edit `project.mcp.json`: update `directory` entries for changed/new files, remove deleted entries
3. Replace the entire `"manifest"` section and `"manifest_md5"` field with the output values

### B. After editing files during this session — update immediately

Whenever you edit a tracked file, update the MCP right away:

1. Read the file you just edited
2. Edit `project.mcp.json`: update the `directory` entry for the file
3. Run `bash bin/update-context.sh`
4. Replace the `"manifest"` section and `"manifest_md5"` field with the output values

This keeps the MCP current at the end of the session, so any next session starts clean.

## Output Format

```
changed: internal/config/policy.go
new: internal/newmodule/module.go
deleted: internal/deprecated/stuff.go
old_manifest_md5: abc123...
new_manifest_json: {"updated": "...", "files": {"internal/config/policy.go": "def456...", ...}}
```

## Replace Pattern

The manifest section in `project.mcp.json` looks like:

```json
"manifest_md5": "...",
"manifest": {
  "updated": "...",
  "files": { ... }
},
```

Replace this entire block (from `"manifest_md5"` through the closing `}`) with the new values from output.

## Notes

- Only tracks non-test `.go` files plus config/metadata files
- Test files are excluded (use existing tests for logic verification)
- Deleted file history preserved in git
- After update, the MCP file is current and ready for use
