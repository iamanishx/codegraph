package parser

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"
)

type TypeScriptParser struct{}

var (
	tsNamedImportRe      = regexp.MustCompile(`import\s*\{([^}]+)\}\s*from\s*['"]([^'"]+)['"]`)
	tsDefaultImportRe    = regexp.MustCompile(`import\s+(\w+)\s+from\s*['"]([^'"]+)['"]`)
	tsStarImportRe       = regexp.MustCompile(`import\s*\*\s*as\s+(\w+)\s+from\s*['"]([^'"]+)['"]`)
	tsSideEffectImportRe = regexp.MustCompile(`import\s+['"]([^'"]+)['"]`)
	tsRequireDestructRe  = regexp.MustCompile(`(?:const|let|var)\s*\{([^}]+)\}\s*=\s*require\s*\(\s*['"]([^'"]+)['"]\s*\)`)
	tsRequireRe          = regexp.MustCompile(`(?:const|let|var)\s+(\w+)\s*=\s*require\s*\(\s*['"]([^'"]+)['"]\s*\)`)

	tsExportFuncRe      = regexp.MustCompile(`export\s+(?:async\s+)?function\s+(\w+)`)
	tsExportClassRe     = regexp.MustCompile(`export\s+class\s+(\w+)`)
	tsExportInterfaceRe = regexp.MustCompile(`export\s+interface\s+(\w+)`)
	tsExportTypeRe      = regexp.MustCompile(`export\s+type\s+(\w+)`)
	tsExportVarRe       = regexp.MustCompile(`export\s+(?:const|let|var)\s+(\w+)`)
	tsExportEnumRe      = regexp.MustCompile(`export\s+enum\s+(\w+)`)
	tsExportDefaultRe   = regexp.MustCompile(`export\s+default\s+(?:class|function|abstract\s+class)\s+(\w+)`)
	tsReExportRe        = regexp.MustCompile(`export\s*\{([^}]+)\}`)
)

func (p *TypeScriptParser) SupportedExtensions() []string {
	return []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"}
}

func (p *TypeScriptParser) Parse(source []byte, filePath string) (*ParseResult, error) {
	result := &ParseResult{}

	scanner := bufio.NewScanner(bytes.NewReader(source))
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			continue
		}

		if m := tsNamedImportRe.FindStringSubmatch(line); m != nil {
			symbols := parseSymbolList(m[1])
			result.Imports = append(result.Imports, ImportInfo{Path: m[2], Symbols: symbols})
			continue
		}
		if m := tsDefaultImportRe.FindStringSubmatch(line); m != nil {
			result.Imports = append(result.Imports, ImportInfo{Path: m[2], Symbols: []string{m[1]}})
			continue
		}
		if m := tsStarImportRe.FindStringSubmatch(line); m != nil {
			result.Imports = append(result.Imports, ImportInfo{Path: m[2], Symbols: []string{"*"}})
			continue
		}
		if m := tsSideEffectImportRe.FindStringSubmatch(line); m != nil {
			result.Imports = append(result.Imports, ImportInfo{Path: m[1], Symbols: []string{"*"}})
			continue
		}
		if m := tsRequireDestructRe.FindStringSubmatch(line); m != nil {
			symbols := parseSymbolList(m[1])
			result.Imports = append(result.Imports, ImportInfo{Path: m[2], Symbols: symbols})
			continue
		}
		if m := tsRequireRe.FindStringSubmatch(line); m != nil {
			result.Imports = append(result.Imports, ImportInfo{Path: m[2], Symbols: []string{m[1]}})
			continue
		}

		if m := tsExportDefaultRe.FindStringSubmatch(line); m != nil {
			result.Exports = append(result.Exports, SymbolInfo{Name: m[1], Kind: "class", Line: lineNum})
			continue
		}
		if m := tsExportFuncRe.FindStringSubmatch(line); m != nil {
			result.Exports = append(result.Exports, SymbolInfo{Name: m[1], Kind: "func", Line: lineNum})
			continue
		}
		if m := tsExportClassRe.FindStringSubmatch(line); m != nil {
			result.Exports = append(result.Exports, SymbolInfo{Name: m[1], Kind: "class", Line: lineNum})
			continue
		}
		if m := tsExportInterfaceRe.FindStringSubmatch(line); m != nil {
			result.Exports = append(result.Exports, SymbolInfo{Name: m[1], Kind: "interface", Line: lineNum})
			continue
		}
		if m := tsExportTypeRe.FindStringSubmatch(line); m != nil {
			result.Exports = append(result.Exports, SymbolInfo{Name: m[1], Kind: "type", Line: lineNum})
			continue
		}
		if m := tsExportVarRe.FindStringSubmatch(line); m != nil {
			result.Exports = append(result.Exports, SymbolInfo{Name: m[1], Kind: "var", Line: lineNum})
			continue
		}
		if m := tsExportEnumRe.FindStringSubmatch(line); m != nil {
			result.Exports = append(result.Exports, SymbolInfo{Name: m[1], Kind: "type", Line: lineNum})
			continue
		}
	}

	return result, scanner.Err()
}

func parseSymbolList(raw string) []string {
	parts := strings.Split(raw, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if idx := strings.Index(p, " as "); idx >= 0 {
			p = strings.TrimSpace(p[idx+4:])
		}
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
