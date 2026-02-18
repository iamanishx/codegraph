# codegraph

A dependency graph MCP server for LLMs. Instead of grepping through your codebase to find related files, codegraph pre-indexes your project into a SQLite database and exposes it as MCP tools. When an LLM needs to understand a file's context, it calls `get_context` and instantly gets back the imports, exports, and importers — no searching required.

## How it works

```
your project files
       │
       ▼
   ┌────────┐     ┌──────────┐     ┌────────┐
   │ Walker  │────▶│ Parsers  │────▶│ SQLite │
   │(fs+git) │     │(TS/Py/Go)│     │   DB   │
   └────────┘     └──────────┘     └────┬───┘
                                        │
                                        ▼
                                  ┌───────────┐
                                  │ MCP Server │◀── LLM calls tools
                                  │  (stdio)   │
                                  └───────────┘
```

1. Walks your codebase (respects `.gitignore`)
2. Parses imports/exports using regex-based extractors for TypeScript/JavaScript, Python, and Go
3. Stores the dependency graph in a SQLite database at `.codegraph/graph.db`
4. Exposes the graph as MCP tools over stdio

## Install

```sh
go install github.com/iamanishx/codegraph/cmd/codegraph@latest
```

Or build from source:

```sh
git clone https://github.com/iamanishx/codegraph.git
cd codegraph
go build -o codegraph ./cmd/codegraph/
```

## Quick start

```sh
cd /path/to/your/project

# initialize the database
codegraph init

# index the codebase
codegraph index

# query a file's context
codegraph query src/api/auth.ts
```

Example output:

```json
{
  "file": "src/api/auth.ts",
  "lang": "typescript",
  "exports": [
    { "name": "AuthConfig", "kind": "interface", "line": 4 },
    { "name": "authenticate", "kind": "func", "line": 9 },
    { "name": "validateToken", "kind": "func", "line": 15 }
  ],
  "imports": {
    "src/db/users.ts": ["findUser", "User"],
    "src/utils/jwt.ts": ["sign", "verify"]
  },
  "importedBy": {
    "src/api/index.ts": ["authenticate", "AuthConfig"],
    "src/routes/login.ts": ["authenticate"],
    "src/routes/middleware.ts": ["validateToken"]
  }
}
```

## MCP tools

The server exposes four tools:

| Tool | Description |
|------|-------------|
| `get_context` | Returns the full dependency context for a file — exports, imports, and importers |
| `get_symbols` | Returns all exported symbols (functions, classes, types, variables) in a file |
| `find_usages` | Finds all files that import a given symbol, optionally scoped to a source file |
| `reindex` | Triggers a full reindex of the codebase |

## Connecting via stdio

codegraph runs as a stdio MCP server. The LLM client launches it as a subprocess and communicates over stdin/stdout using JSON-RPC.

The server automatically discovers the project root by walking up from the current working directory to find `.codegraph/` or `.git/`. Since MCP clients spawn the subprocess with cwd set to the project root, you only need one global config — no per-project setup.

Start the server manually:

```sh
cd /path/to/your/project
codegraph serve
```

If you need to override the project root:

```sh
codegraph serve --root /path/to/your/project
```

### OpenCode

Add to your `opencode.json` (or `opencode.jsonc`):

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "codegraph": {
      "type": "local",
      "command": ["codegraph", "serve"],
      "enabled": true
    }
  }
}
```

### Claude Desktop

Add to your `claude_desktop_config.json`:

- macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
- Windows: `%APPDATA%\Claude\claude_desktop_config.json`

```json
{
  "mcpServers": {
    "codegraph": {
      "command": "codegraph",
      "args": ["serve"]
    }
  }
}
```

### Cursor

Add to `.cursor/mcp.json` in your project root (or `~/.cursor/mcp.json` for global):

```json
{
  "mcpServers": {
    "codegraph": {
      "command": "codegraph",
      "args": ["serve"]
    }
  }
}
```

### VS Code

Add to your MCP config (`Cmd+Shift+P` > `MCP: Open User Configuration`):

```json
{
  "servers": {
    "codegraph": {
      "type": "stdio",
      "command": "codegraph",
      "args": ["serve"]
    }
  }
}
```

### Cline / Roo Code / Kilocode

Add to the MCP settings JSON:

```json
{
  "mcpServers": {
    "codegraph": {
      "type": "stdio",
      "command": "codegraph",
      "args": ["serve"]
    }
  }
}
```

### Claude Code

```sh
claude mcp add codegraph -- codegraph serve
```

### Any MCP client

The server speaks standard MCP over stdio. Launch `codegraph serve` as a subprocess, pipe stdin/stdout, and send JSON-RPC messages per the [MCP spec](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports#stdio).

## Keeping the index in sync

### Git hook (recommended)

Install a post-commit hook that incrementally re-indexes only changed files:

```sh
codegraph install-hook
```

This creates `.git/hooks/post-commit` that runs `codegraph diff` after each commit.

### Manual

```sh
# full reindex
codegraph index

# incremental — only files changed in the last commit
codegraph diff
```

### Via MCP

The LLM can call the `reindex` tool directly if the graph seems stale.

## CLI reference

```
codegraph init                          initialize .codegraph/graph.db in the current project
codegraph index                         full reindex of the codebase
codegraph query <file>                  print the context JSON for a file
codegraph serve [--root /path/to/proj]  start the MCP server (stdio)
codegraph install-hook                  install a git post-commit hook
codegraph diff                          index only files changed in the last commit
codegraph stats                         print file/symbol/edge counts
```

## Supported languages

| Language | Extensions | What's extracted |
|----------|-----------|-----------------|
| TypeScript/JavaScript | `.ts` `.tsx` `.js` `.jsx` `.mjs` `.cjs` | ES imports, require(), named/default exports, re-exports |
| Python | `.py` `.pyi` | `from x import y`, `import x`, top-level functions/classes/constants |
| Go | `.go` | import blocks, exported functions/methods/types/vars/consts |

## Performance

- 875 files, 3508 symbols, 5835 edges indexed in ~400ms
- Full reindex of unchanged files: skipped via content hash (< 1ms)
- Incremental index (1 changed file): ~50ms
- Single file query: < 1ms
- Concurrent parsing with goroutine worker pool
- All DB writes batched in a single transaction

## Project structure

```
codegraph/
├── cmd/codegraph/  CLI entry point
├── db/             SQLite schema and data access
├── parser/         per-language import/export extractors
├── walker/         filesystem walker (.gitignore aware)
├── indexer/        orchestrates walk → parse → store (concurrent)
├── query/          builds context JSON from the graph
└── server/         MCP server (stdio transport)
```
