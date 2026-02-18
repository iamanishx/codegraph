package parser

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"
)

type PythonParser struct{}

var (
	pyFromImportRe = regexp.MustCompile(`from\s+([\w.]+)\s+import\s+(.+)`)
	pyImportRe     = regexp.MustCompile(`^import\s+([\w.]+(?:\s*,\s*[\w.]+)*)`)

	pyFuncRe  = regexp.MustCompile(`^def\s+(\w+)\s*\(`)
	pyClassRe = regexp.MustCompile(`^class\s+(\w+)`)
	pyConstRe = regexp.MustCompile(`^([A-Z][A-Z_0-9]+)\s*=`)
	pyVarRe   = regexp.MustCompile(`^(\w+)\s*(?::\s*\w+\s*)?=`)
)

func (p *PythonParser) SupportedExtensions() []string {
	return []string{".py", ".pyi"}
}

func (p *PythonParser) Parse(source []byte, filePath string) (*ParseResult, error) {
	result := &ParseResult{}

	scanner := bufio.NewScanner(bytes.NewReader(source))
	lineNum := 0
	inDocstring := false

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.Count(trimmed, `"""`) == 1 || strings.Count(trimmed, `'''`) == 1 {
			inDocstring = !inDocstring
			continue
		}
		if inDocstring {
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		if m := pyFromImportRe.FindStringSubmatch(trimmed); m != nil {
			modPath := m[1]
			importPart := m[2]

			importPart = strings.TrimSuffix(importPart, "\\")
			importPart = strings.Trim(importPart, "()")

			symbols := parsePythonImportList(importPart)
			result.Imports = append(result.Imports, ImportInfo{
				Path:    strings.ReplaceAll(modPath, ".", "/"),
				Symbols: symbols,
			})
			continue
		}
		if m := pyImportRe.FindStringSubmatch(trimmed); m != nil {
			mods := strings.Split(m[1], ",")
			for _, mod := range mods {
				mod = strings.TrimSpace(mod)
				if idx := strings.Index(mod, " as "); idx >= 0 {
					mod = strings.TrimSpace(mod[:idx])
				}
				result.Imports = append(result.Imports, ImportInfo{
					Path:    strings.ReplaceAll(mod, ".", "/"),
					Symbols: []string{"*"},
				})
			}
			continue
		}

		if len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
			if m := pyFuncRe.FindStringSubmatch(trimmed); m != nil {
				if !strings.HasPrefix(m[1], "_") {
					result.Exports = append(result.Exports, SymbolInfo{Name: m[1], Kind: "func", Line: lineNum})
				}
				continue
			}
			if m := pyClassRe.FindStringSubmatch(trimmed); m != nil {
				if !strings.HasPrefix(m[1], "_") {
					result.Exports = append(result.Exports, SymbolInfo{Name: m[1], Kind: "class", Line: lineNum})
				}
				continue
			}
			if m := pyConstRe.FindStringSubmatch(trimmed); m != nil {
				result.Exports = append(result.Exports, SymbolInfo{Name: m[1], Kind: "const", Line: lineNum})
				continue
			}
		}
	}

	return result, scanner.Err()
}

func parsePythonImportList(raw string) []string {
	parts := strings.Split(raw, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if idx := strings.Index(p, " as "); idx >= 0 {
			p = strings.TrimSpace(p[idx+4:])
		}
		if p == "*" {
			result = append(result, "*")
		} else if p != "" {
			result = append(result, p)
		}
	}
	return result
}
