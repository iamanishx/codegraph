package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/iamanishx/codegraph/db"
	"github.com/iamanishx/codegraph/indexer"
	"github.com/iamanishx/codegraph/query"
	"github.com/iamanishx/codegraph/server"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]

	switch cmd {
	case "init":
		cmdInit()
	case "index":
		cmdIndex()
	case "query":
		cmdQuery()
	case "serve":
		cmdServe()
	case "install-hook":
		cmdInstallHook()
	case "diff":
		cmdDiff()
	case "stats":
		cmdStats()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `codegraph - dependency graph for LLMs

usage:
  codegraph init                    initialize graph db in current project
  codegraph index                   full reindex of the codebase
  codegraph query <file>            get context JSON for a file
  codegraph serve                   start MCP server (stdio)
  codegraph install-hook            install git post-commit hook
  codegraph diff                    index only git-changed files
  codegraph stats                   show index statistics
`)
}

func getRoot() string {
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	return root
}

func openDB(root string) *db.DB {
	dbPath := filepath.Join(root, ".codegraph", "graph.db")
	database, err := db.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening database: %v\n", err)
		os.Exit(1)
	}
	return database
}

func cmdInit() {
	root := getRoot()
	dbPath := filepath.Join(root, ".codegraph", "graph.db")
	database, err := db.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	database.Close()

	gitignorePath := filepath.Join(root, ".codegraph", ".gitignore")
	os.WriteFile(gitignorePath, []byte("graph.db\ngraph.db-wal\ngraph.db-shm\n"), 0644)

	fmt.Printf("initialized codegraph at %s\n", filepath.Join(root, ".codegraph"))
}

func cmdIndex() {
	root := getRoot()
	database := openDB(root)
	defer database.Close()

	idx := indexer.New(database, root)
	stats, err := idx.IndexAll()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("indexed %d files, %d symbols, %d edges in %s\n",
		stats.Files, stats.Symbols, stats.Edges, stats.Duration)
}

func cmdQuery() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "usage: codegraph query <file>\n")
		os.Exit(1)
	}

	root := getRoot()
	database := openDB(root)
	defer database.Close()

	filePath := os.Args[2]
	eng := query.New(database)
	result, err := eng.GetContextJSON(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(result)
}

func cmdServe() {
	root := getRoot()
	srv, err := server.New(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer srv.Close()

	if err := srv.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func cmdInstallHook() {
	root := getRoot()
	hookDir := filepath.Join(root, ".git", "hooks")

	if _, err := os.Stat(hookDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "error: .git/hooks directory not found. is this a git repo?\n")
		os.Exit(1)
	}

	hookContent := `#!/bin/sh
codegraph diff
`
	hookPath := filepath.Join(hookDir, "post-commit")
	if err := os.WriteFile(hookPath, []byte(hookContent), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error writing hook: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("installed post-commit hook at %s\n", hookPath)
}

func cmdDiff() {
	root := getRoot()

	out, err := exec.Command("git", "diff", "--name-only", "HEAD~1", "HEAD").Output()
	if err != nil {
		out, err = exec.Command("git", "diff", "--name-only", "--cached").Output()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error getting git diff: %v\n", err)
			os.Exit(1)
		}
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var files []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}

	if len(files) == 0 {
		fmt.Println("no changed files to index")
		return
	}

	database := openDB(root)
	defer database.Close()

	idx := indexer.New(database, root)
	stats, err := idx.IndexFiles(files)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("incrementally indexed %d files, %d symbols, %d edges in %s\n",
		stats.Files, stats.Symbols, stats.Edges, stats.Duration)
}

func cmdStats() {
	root := getRoot()
	database := openDB(root)
	defer database.Close()

	files, symbols, edges, err := database.Stats()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("files: %d\nsymbols: %d\nedges: %d\n", files, symbols, edges)
}
