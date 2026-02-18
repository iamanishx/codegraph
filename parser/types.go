package parser

type ParseResult struct {
	Exports []SymbolInfo
	Imports []ImportInfo
}

type SymbolInfo struct {
	Name string
	Kind string
	Line int
}

type ImportInfo struct {
	Path    string
	Symbols []string
}

type Parser interface {
	Parse(source []byte, filePath string) (*ParseResult, error)
	SupportedExtensions() []string
}
