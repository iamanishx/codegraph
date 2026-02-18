package indexer

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/manish/codegraph/db"
	"github.com/manish/codegraph/parser"
	"github.com/manish/codegraph/walker"
)

type Indexer struct {
	db   *db.DB
	root string
}

type Stats struct {
	Files    int
	Symbols  int
	Edges    int
	Duration time.Duration
}

func New(database *db.DB, root string) *Indexer {
	return &Indexer{db: database, root: root}
}

type parseResult struct {
	entry   walker.FileEntry
	symbols []db.SymbolWriteOp
	edges   []db.EdgeWriteOp
}

func (idx *Indexer) IndexAll() (*Stats, error) {
	start := time.Now()

	w := walker.New(idx.root)
	entries, err := w.Walk()
	if err != nil {
		return nil, fmt.Errorf("walking filesystem: %w", err)
	}

	existingHashes, err := idx.db.GetAllFileHashes()
	if err != nil {
		return nil, fmt.Errorf("loading existing hashes: %w", err)
	}

	var toProcess []walker.FileEntry
	for _, entry := range entries {
		if hash, ok := existingHashes[entry.RelPath]; ok && hash == entry.ContentHash {
			continue
		}
		toProcess = append(toProcess, entry)
	}

	if len(toProcess) == 0 {
		return &Stats{Duration: time.Since(start)}, nil
	}

	workers := runtime.NumCPU()
	if workers > 16 {
		workers = 16
	}
	if workers > len(toProcess) {
		workers = len(toProcess)
	}

	jobs := make(chan walker.FileEntry, len(toProcess))
	results := make(chan parseResult, len(toProcess))

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for entry := range jobs {
				pr := parseEntry(entry)
				if pr != nil {
					results <- *pr
				}
			}
		}()
	}

	for _, entry := range toProcess {
		jobs <- entry
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
	}()

	var ops []db.FileWriteOp
	for pr := range results {
		ops = append(ops, db.FileWriteOp{
			RelPath:     pr.entry.RelPath,
			Lang:        pr.entry.Lang,
			ContentHash: pr.entry.ContentHash,
			Symbols:     pr.symbols,
			Edges:       pr.edges,
		})
	}

	files, symbols, edges, err := idx.db.BatchWrite(ops)
	if err != nil {
		return nil, fmt.Errorf("batch write: %w", err)
	}

	if err := idx.db.ResolveEdges(); err != nil {
		return nil, fmt.Errorf("resolving edges: %w", err)
	}

	return &Stats{
		Files:    files,
		Symbols:  symbols,
		Edges:    edges,
		Duration: time.Since(start),
	}, nil
}

func parseEntry(entry walker.FileEntry) *parseResult {
	p, err := parser.ForFile(entry.RelPath)
	if err != nil {
		return nil
	}

	result, err := p.Parse(entry.Content, entry.RelPath)
	if err != nil {
		return nil
	}

	pr := &parseResult{entry: entry}

	for _, sym := range result.Exports {
		pr.symbols = append(pr.symbols, db.SymbolWriteOp{
			Name: sym.Name,
			Kind: sym.Kind,
			Line: sym.Line,
		})
	}

	for _, imp := range result.Imports {
		resolvedPath := resolveImportPath(entry.RelPath, imp.Path)
		for _, sym := range imp.Symbols {
			pr.edges = append(pr.edges, db.EdgeWriteOp{
				TargetPath: resolvedPath,
				SymbolName: sym,
				Kind:       "import",
			})
		}
	}

	return pr
}

func (idx *Indexer) IndexFiles(paths []string) (*Stats, error) {
	start := time.Now()

	var toProcess []walker.FileEntry
	for _, path := range paths {
		rel, err := filepath.Rel(idx.root, path)
		if err != nil {
			rel = path
		}

		content, err := readFile(filepath.Join(idx.root, rel))
		if err != nil {
			idx.db.DeleteFile(rel)
			continue
		}

		ext := strings.ToLower(filepath.Ext(rel))
		toProcess = append(toProcess, walker.FileEntry{
			Path:        filepath.Join(idx.root, rel),
			RelPath:     rel,
			Lang:        parser.LangForExt(ext),
			ContentHash: contentHash(content),
			Content:     content,
		})
	}

	if len(toProcess) == 0 {
		return &Stats{Duration: time.Since(start)}, nil
	}

	workers := runtime.NumCPU()
	if workers > len(toProcess) {
		workers = len(toProcess)
	}

	jobs := make(chan walker.FileEntry, len(toProcess))
	results := make(chan parseResult, len(toProcess))

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for entry := range jobs {
				pr := parseEntry(entry)
				if pr != nil {
					results <- *pr
				}
			}
		}()
	}

	for _, entry := range toProcess {
		jobs <- entry
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
	}()

	var ops []db.FileWriteOp
	for pr := range results {
		ops = append(ops, db.FileWriteOp{
			RelPath:     pr.entry.RelPath,
			Lang:        pr.entry.Lang,
			ContentHash: pr.entry.ContentHash,
			Symbols:     pr.symbols,
			Edges:       pr.edges,
		})
	}

	files, symbols, edges, err := idx.db.BatchWrite(ops)
	if err != nil {
		return nil, err
	}

	if err := idx.db.ResolveEdges(); err != nil {
		return nil, err
	}

	return &Stats{
		Files:    files,
		Symbols:  symbols,
		Edges:    edges,
		Duration: time.Since(start),
	}, nil
}

func resolveImportPath(sourceFile, importPath string) string {
	if strings.HasPrefix(importPath, ".") {
		dir := filepath.Dir(sourceFile)
		resolved := filepath.Join(dir, importPath)
		resolved = filepath.Clean(resolved)
		return resolved
	}
	return importPath
}

func readFile(path string) ([]byte, error) {
	return readFileBytes(path)
}
