package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/manish/codegraph/db"
	"github.com/manish/codegraph/indexer"
	"github.com/manish/codegraph/query"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Server struct {
	db      *db.DB
	query   *query.Engine
	indexer *indexer.Indexer
	root    string
}

func New(root string) (*Server, error) {
	dbPath := filepath.Join(root, ".codegraph", "graph.db")
	database, err := db.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	return &Server{
		db:      database,
		query:   query.New(database),
		indexer: indexer.New(database, root),
		root:    root,
	}, nil
}

func (s *Server) Close() error {
	return s.db.Close()
}

type getContextArgs struct {
	FilePath string `json:"file_path" jsonschema:"description=Relative path to the file from project root"`
}

type getSymbolsArgs struct {
	FilePath string `json:"file_path" jsonschema:"description=Relative path to the file from project root"`
}

type findUsagesArgs struct {
	Symbol   string `json:"symbol" jsonschema:"description=Name of the symbol to find usages for"`
	FilePath string `json:"file_path,omitempty" jsonschema:"description=Optional: scope search to usages of this symbol from a specific file"`
}

type reindexArgs struct{}

func textResult(text string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}, nil, nil
}

func (s *Server) Run() error {
	srv := mcp.NewServer(
		&mcp.Implementation{
			Name:    "codegraph",
			Version: "0.1.0",
		},
		nil,
	)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_context",
		Description: "Get the dependency context for a file: its exports, what it imports, and what imports it. Returns a compact JSON graph.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getContextArgs) (*mcp.CallToolResult, any, error) {
		result, err := s.query.GetContextJSON(args.FilePath)
		if err != nil {
			return nil, nil, err
		}
		if result == "{}" {
			return textResult(fmt.Sprintf("File not found in index: %s. Try running reindex first.", args.FilePath))
		}
		return textResult(result)
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_symbols",
		Description: "Get all exported symbols (functions, classes, types, variables) defined in a file.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args getSymbolsArgs) (*mcp.CallToolResult, any, error) {
		symbols, err := s.query.GetSymbols(args.FilePath)
		if err != nil {
			return nil, nil, err
		}
		data, err := json.MarshalIndent(symbols, "", "  ")
		if err != nil {
			return nil, nil, err
		}
		return textResult(string(data))
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "find_usages",
		Description: "Find all files that import/use a given symbol. Optionally scope to usages from a specific source file.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args findUsagesArgs) (*mcp.CallToolResult, any, error) {
		var filePath *string
		if args.FilePath != "" {
			filePath = &args.FilePath
		}
		result, err := s.query.FindUsages(args.Symbol, filePath)
		if err != nil {
			return nil, nil, err
		}
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return nil, nil, err
		}
		return textResult(string(data))
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "reindex",
		Description: "Re-index the entire codebase. Run this after major changes or if the graph seems stale.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args reindexArgs) (*mcp.CallToolResult, any, error) {
		stats, err := s.indexer.IndexAll()
		if err != nil {
			return nil, nil, err
		}
		msg := fmt.Sprintf("Indexed %d files, %d symbols, %d edges in %s",
			stats.Files, stats.Symbols, stats.Edges, stats.Duration)
		return textResult(msg)
	})

	transport := &mcp.StdioTransport{}

	fmt.Fprintf(os.Stderr, "codegraph MCP server starting (root: %s)\n", s.root)
	return srv.Run(context.Background(), transport)
}
