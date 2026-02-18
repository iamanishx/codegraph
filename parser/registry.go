package parser

import (
	"fmt"
	"path/filepath"
	"strings"
)

var registry = map[string]Parser{}

func init() {
	ts := &TypeScriptParser{}
	for _, ext := range ts.SupportedExtensions() {
		registry[ext] = ts
	}

	py := &PythonParser{}
	for _, ext := range py.SupportedExtensions() {
		registry[ext] = py
	}

	gop := &GoParser{}
	for _, ext := range gop.SupportedExtensions() {
		registry[ext] = gop
	}
}

func ForFile(filePath string) (Parser, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	p, ok := registry[ext]
	if !ok {
		return nil, fmt.Errorf("unsupported file extension: %s", ext)
	}
	return p, nil
}

func SupportedExtensions() []string {
	exts := make([]string, 0, len(registry))
	for ext := range registry {
		exts = append(exts, ext)
	}
	return exts
}

func LangForExt(ext string) string {
	switch ext {
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".py", ".pyi":
		return "python"
	case ".go":
		return "go"
	default:
		return "unknown"
	}
}
