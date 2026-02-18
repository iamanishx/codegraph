package walker

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/manish/codegraph/parser"
)

type FileEntry struct {
	Path        string
	RelPath     string
	Lang        string
	ContentHash string
	Content     []byte
}

type Walker struct {
	root       string
	ignoreSet  map[string]bool
	extensions map[string]bool
}

func New(root string) *Walker {
	exts := make(map[string]bool)
	for _, e := range parser.SupportedExtensions() {
		exts[e] = true
	}
	return &Walker{
		root:       root,
		ignoreSet:  defaultIgnores(),
		extensions: exts,
	}
}

func (w *Walker) Walk() ([]FileEntry, error) {
	w.loadGitignore()

	var entries []FileEntry
	err := filepath.Walk(w.root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		rel, _ := filepath.Rel(w.root, path)

		if info.IsDir() {
			if w.shouldIgnoreDir(rel, info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !w.extensions[ext] {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		hash := sha256.Sum256(content)

		entries = append(entries, FileEntry{
			Path:        path,
			RelPath:     rel,
			Lang:        parser.LangForExt(ext),
			ContentHash: hex.EncodeToString(hash[:]),
			Content:     content,
		})

		return nil
	})

	return entries, err
}

func (w *Walker) shouldIgnoreDir(rel, name string) bool {
	if strings.HasPrefix(name, ".") && name != "." {
		return true
	}
	if w.ignoreSet[name] {
		return true
	}
	for pattern := range w.ignoreSet {
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, rel); matched {
			return true
		}
	}
	return false
}

func (w *Walker) loadGitignore() {
	f, err := os.Open(filepath.Join(w.root, ".gitignore"))
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSuffix(line, "/")
		w.ignoreSet[line] = true
	}
}

func defaultIgnores() map[string]bool {
	return map[string]bool{
		"node_modules":  true,
		"vendor":        true,
		"__pycache__":   true,
		".venv":         true,
		"venv":          true,
		"dist":          true,
		"build":         true,
		".git":          true,
		".next":         true,
		".nuxt":         true,
		"coverage":      true,
		".tox":          true,
		"env":           true,
		".env":          true,
		".mypy_cache":   true,
		".pytest_cache": true,
		"__snapshots__": true,
		".turbo":        true,
		".vercel":       true,
	}
}
