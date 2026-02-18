package query

import (
	"encoding/json"

	"github.com/manish/codegraph/db"
)

type Engine struct {
	db *db.DB
}

type ContextResult struct {
	File       string              `json:"file"`
	Lang       string              `json:"lang"`
	Exports    []ExportEntry       `json:"exports"`
	Imports    map[string][]string `json:"imports"`
	ImportedBy map[string][]string `json:"importedBy"`
}

type ExportEntry struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	Line int    `json:"line"`
}

type SymbolUsageResult struct {
	Symbol string   `json:"symbol"`
	File   string   `json:"file,omitempty"`
	UsedBy []string `json:"usedBy"`
}

func New(database *db.DB) *Engine {
	return &Engine{db: database}
}

func (e *Engine) GetContext(filePath string) (*ContextResult, error) {
	file, err := e.db.GetFileByPath(filePath)
	if err != nil {
		return nil, err
	}
	if file == nil {
		return nil, nil
	}

	result := &ContextResult{
		File: file.Path,
		Lang: file.Lang,
	}

	symbols, err := e.db.GetSymbols(file.ID)
	if err != nil {
		return nil, err
	}
	for _, s := range symbols {
		result.Exports = append(result.Exports, ExportEntry{
			Name: s.Name,
			Kind: s.Kind,
			Line: s.Line,
		})
	}

	imports, err := e.db.GetImports(file.ID)
	if err != nil {
		return nil, err
	}
	result.Imports = imports

	importers, err := e.db.GetImporters(file.ID)
	if err != nil {
		return nil, err
	}
	result.ImportedBy = importers

	return result, nil
}

func (e *Engine) GetSymbols(filePath string) ([]ExportEntry, error) {
	file, err := e.db.GetFileByPath(filePath)
	if err != nil {
		return nil, err
	}
	if file == nil {
		return nil, nil
	}

	symbols, err := e.db.GetSymbols(file.ID)
	if err != nil {
		return nil, err
	}

	var entries []ExportEntry
	for _, s := range symbols {
		entries = append(entries, ExportEntry{
			Name: s.Name,
			Kind: s.Kind,
			Line: s.Line,
		})
	}
	return entries, nil
}

func (e *Engine) FindUsages(symbolName string, filePath *string) (*SymbolUsageResult, error) {
	var fileID *int64
	if filePath != nil {
		file, err := e.db.GetFileByPath(*filePath)
		if err != nil {
			return nil, err
		}
		if file != nil {
			fileID = &file.ID
		}
	}

	usages, err := e.db.FindSymbolUsages(symbolName, fileID)
	if err != nil {
		return nil, err
	}

	result := &SymbolUsageResult{
		Symbol: symbolName,
		UsedBy: usages,
	}
	if filePath != nil {
		result.File = *filePath
	}
	return result, nil
}

func (e *Engine) GetContextJSON(filePath string) (string, error) {
	ctx, err := e.GetContext(filePath)
	if err != nil {
		return "", err
	}
	if ctx == nil {
		return "{}", nil
	}

	data, err := json.MarshalIndent(ctx, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
