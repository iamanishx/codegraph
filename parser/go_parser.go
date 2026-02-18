package parser

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"
)

type GoParser struct{}

var (
	goSingleImportRe    = regexp.MustCompile(`import\s+(?:(\w+)\s+)?["']([^"']+)["']`)
	goGroupImportLineRe = regexp.MustCompile(`^\s*(?:(\w+)\s+)?["']([^"']+)["']`)

	goFuncRe   = regexp.MustCompile(`^func\s+(\w+)\s*[\[(]`)
	goMethodRe = regexp.MustCompile(`^func\s+\([^)]+\)\s+(\w+)\s*[\[(]`)
	goTypeRe   = regexp.MustCompile(`^type\s+(\w+)\s+`)
	goVarRe    = regexp.MustCompile(`^var\s+(\w+)\s+`)
	goConstRe  = regexp.MustCompile(`^const\s+(\w+)\s+`)
)

func (p *GoParser) SupportedExtensions() []string {
	return []string{".go"}
}

func (p *GoParser) Parse(source []byte, filePath string) (*ParseResult, error) {
	result := &ParseResult{}

	scanner := bufio.NewScanner(bytes.NewReader(source))
	lineNum := 0
	inImportBlock := false

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		if strings.HasPrefix(trimmed, "import (") {
			inImportBlock = true
			continue
		}
		if inImportBlock {
			if trimmed == ")" {
				inImportBlock = false
				continue
			}
			if m := goGroupImportLineRe.FindStringSubmatch(line); m != nil {
				result.Imports = append(result.Imports, ImportInfo{
					Path:    m[2],
					Symbols: []string{"*"},
				})
			}
			continue
		}

		if strings.HasPrefix(trimmed, "import ") && !strings.Contains(trimmed, "(") {
			if m := goSingleImportRe.FindStringSubmatch(line); m != nil {
				result.Imports = append(result.Imports, ImportInfo{
					Path:    m[2],
					Symbols: []string{"*"},
				})
			}
			continue
		}

		if m := goMethodRe.FindStringSubmatch(trimmed); m != nil {
			if isExported(m[1]) {
				result.Exports = append(result.Exports, SymbolInfo{Name: m[1], Kind: "func", Line: lineNum})
			}
			continue
		}
		if m := goFuncRe.FindStringSubmatch(trimmed); m != nil {
			if isExported(m[1]) {
				result.Exports = append(result.Exports, SymbolInfo{Name: m[1], Kind: "func", Line: lineNum})
			}
			continue
		}
		if m := goTypeRe.FindStringSubmatch(trimmed); m != nil {
			if isExported(m[1]) {
				kind := "type"
				if strings.Contains(trimmed, "interface") {
					kind = "interface"
				}
				result.Exports = append(result.Exports, SymbolInfo{Name: m[1], Kind: kind, Line: lineNum})
			}
			continue
		}
		if m := goVarRe.FindStringSubmatch(trimmed); m != nil {
			if isExported(m[1]) {
				result.Exports = append(result.Exports, SymbolInfo{Name: m[1], Kind: "var", Line: lineNum})
			}
			continue
		}
		if m := goConstRe.FindStringSubmatch(trimmed); m != nil {
			if isExported(m[1]) {
				result.Exports = append(result.Exports, SymbolInfo{Name: m[1], Kind: "const", Line: lineNum})
			}
			continue
		}
	}

	return result, scanner.Err()
}

func isExported(name string) bool {
	if len(name) == 0 {
		return false
	}
	return name[0] >= 'A' && name[0] <= 'Z'
}
