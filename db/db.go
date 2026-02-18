package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const schemaSQL = `
CREATE TABLE IF NOT EXISTS files (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	path          TEXT NOT NULL UNIQUE,
	lang          TEXT NOT NULL,
	content_hash  TEXT NOT NULL,
	last_indexed  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS symbols (
	id       INTEGER PRIMARY KEY AUTOINCREMENT,
	file_id  INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
	name     TEXT NOT NULL,
	kind     TEXT NOT NULL,
	line     INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS edges (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	source_file_id  INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
	target_path     TEXT NOT NULL,
	target_file_id  INTEGER REFERENCES files(id) ON DELETE SET NULL,
	symbol_name     TEXT NOT NULL DEFAULT '*',
	kind            TEXT NOT NULL DEFAULT 'import'
);

CREATE INDEX IF NOT EXISTS idx_files_path ON files(path);
CREATE INDEX IF NOT EXISTS idx_files_hash ON files(content_hash);
CREATE INDEX IF NOT EXISTS idx_symbols_file ON symbols(file_id);
CREATE INDEX IF NOT EXISTS idx_symbols_name ON symbols(name);
CREATE INDEX IF NOT EXISTS idx_edges_source ON edges(source_file_id);
CREATE INDEX IF NOT EXISTS idx_edges_target ON edges(target_file_id);
CREATE INDEX IF NOT EXISTS idx_edges_target_path ON edges(target_path);
`

type DB struct {
	conn *sql.DB
	path string
}

func Open(dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating db directory: %w", err)
	}

	conn, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	if _, err := conn.Exec(schemaSQL); err != nil {
		conn.Close()
		return nil, fmt.Errorf("initializing schema: %w", err)
	}

	return &DB{conn: conn, path: dbPath}, nil
}

func (d *DB) Close() error {
	return d.conn.Close()
}

type File struct {
	ID          int64
	Path        string
	Lang        string
	ContentHash string
}

type Symbol struct {
	ID     int64
	FileID int64
	Name   string
	Kind   string
	Line   int
}

type Edge struct {
	ID           int64
	SourceFileID int64
	TargetPath   string
	TargetFileID sql.NullInt64
	SymbolName   string
	Kind         string
}

func (d *DB) UpsertFile(path, lang, contentHash string) (int64, error) {
	_, err := d.conn.Exec(`
		INSERT INTO files (path, lang, content_hash, last_indexed)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(path) DO UPDATE SET
			lang = excluded.lang,
			content_hash = excluded.content_hash,
			last_indexed = CURRENT_TIMESTAMP
	`, path, lang, contentHash)
	if err != nil {
		return 0, err
	}

	var id int64
	err = d.conn.QueryRow("SELECT id FROM files WHERE path = ?", path).Scan(&id)
	return id, err
}

func (d *DB) GetFileByPath(path string) (*File, error) {
	f := &File{}
	err := d.conn.QueryRow("SELECT id, path, lang, content_hash FROM files WHERE path = ?", path).
		Scan(&f.ID, &f.Path, &f.Lang, &f.ContentHash)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return f, err
}

func (d *DB) DeleteFileSymbols(fileID int64) error {
	_, err := d.conn.Exec("DELETE FROM symbols WHERE file_id = ?", fileID)
	return err
}

func (d *DB) DeleteFileEdges(fileID int64) error {
	_, err := d.conn.Exec("DELETE FROM edges WHERE source_file_id = ?", fileID)
	return err
}

func (d *DB) InsertSymbol(fileID int64, name, kind string, line int) error {
	_, err := d.conn.Exec(
		"INSERT INTO symbols (file_id, name, kind, line) VALUES (?, ?, ?, ?)",
		fileID, name, kind, line,
	)
	return err
}

func (d *DB) InsertEdge(sourceFileID int64, targetPath, symbolName, kind string) error {
	_, err := d.conn.Exec(
		"INSERT INTO edges (source_file_id, target_path, symbol_name, kind) VALUES (?, ?, ?, ?)",
		sourceFileID, targetPath, symbolName, kind,
	)
	return err
}

func (d *DB) ResolveEdges() error {
	_, err := d.conn.Exec(`
		UPDATE edges SET target_file_id = (
			SELECT f.id FROM files f WHERE
				f.path = edges.target_path
				OR f.path = edges.target_path || '.ts'
				OR f.path = edges.target_path || '.tsx'
				OR f.path = edges.target_path || '.js'
				OR f.path = edges.target_path || '.jsx'
				OR f.path = edges.target_path || '.py'
				OR f.path = edges.target_path || '.go'
				OR f.path = edges.target_path || '/index.ts'
				OR f.path = edges.target_path || '/index.js'
				OR f.path = edges.target_path || '/index.tsx'
			LIMIT 1
		) WHERE target_file_id IS NULL
	`)
	return err
}

func (d *DB) DeleteFile(path string) error {
	_, err := d.conn.Exec("DELETE FROM files WHERE path = ?", path)
	return err
}

func (d *DB) GetImports(fileID int64) (map[string][]string, error) {
	rows, err := d.conn.Query(`
		SELECT COALESCE(f.path, e.target_path), e.symbol_name
		FROM edges e
		LEFT JOIN files f ON f.id = e.target_file_id
		WHERE e.source_file_id = ?
		ORDER BY COALESCE(f.path, e.target_path)
	`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]string)
	for rows.Next() {
		var path, sym string
		if err := rows.Scan(&path, &sym); err != nil {
			return nil, err
		}
		result[path] = append(result[path], sym)
	}
	return result, rows.Err()
}

func (d *DB) GetImporters(fileID int64) (map[string][]string, error) {
	rows, err := d.conn.Query(`
		SELECT f.path, e.symbol_name
		FROM edges e
		JOIN files f ON f.id = e.source_file_id
		WHERE e.target_file_id = ?
		ORDER BY f.path
	`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]string)
	for rows.Next() {
		var path, sym string
		if err := rows.Scan(&path, &sym); err != nil {
			return nil, err
		}
		result[path] = append(result[path], sym)
	}
	return result, rows.Err()
}

func (d *DB) GetSymbols(fileID int64) ([]Symbol, error) {
	rows, err := d.conn.Query(
		"SELECT id, file_id, name, kind, line FROM symbols WHERE file_id = ? ORDER BY line",
		fileID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var syms []Symbol
	for rows.Next() {
		var s Symbol
		if err := rows.Scan(&s.ID, &s.FileID, &s.Name, &s.Kind, &s.Line); err != nil {
			return nil, err
		}
		syms = append(syms, s)
	}
	return syms, rows.Err()
}

func (d *DB) FindSymbolUsages(symbolName string, fileID *int64) ([]string, error) {
	var rows *sql.Rows
	var err error

	if fileID != nil {
		rows, err = d.conn.Query(`
			SELECT DISTINCT f.path
			FROM edges e
			JOIN files f ON f.id = e.source_file_id
			WHERE e.symbol_name = ? AND e.target_file_id = ?
			ORDER BY f.path
		`, symbolName, *fileID)
	} else {
		rows, err = d.conn.Query(`
			SELECT DISTINCT f.path
			FROM edges e
			JOIN files f ON f.id = e.source_file_id
			WHERE e.symbol_name = ?
			ORDER BY f.path
		`, symbolName)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, rows.Err()
}

func (d *DB) BeginTx() (*sql.Tx, error) {
	return d.conn.Begin()
}

func (d *DB) GetAllFileHashes() (map[string]string, error) {
	rows, err := d.conn.Query("SELECT path, content_hash FROM files")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var path, hash string
		if err := rows.Scan(&path, &hash); err != nil {
			return nil, err
		}
		result[path] = hash
	}
	return result, rows.Err()
}

type FileWriteOp struct {
	RelPath     string
	Lang        string
	ContentHash string
	Symbols     []SymbolWriteOp
	Edges       []EdgeWriteOp
}

type SymbolWriteOp struct {
	Name string
	Kind string
	Line int
}

type EdgeWriteOp struct {
	TargetPath string
	SymbolName string
	Kind       string
}

func (d *DB) BatchWrite(ops []FileWriteOp) (files, symbols, edges int, err error) {
	tx, err := d.conn.Begin()
	if err != nil {
		return 0, 0, 0, err
	}
	defer tx.Rollback()

	upsertFile, err := tx.Prepare(`
		INSERT INTO files (path, lang, content_hash, last_indexed)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(path) DO UPDATE SET
			lang = excluded.lang,
			content_hash = excluded.content_hash,
			last_indexed = CURRENT_TIMESTAMP
	`)
	if err != nil {
		return 0, 0, 0, err
	}
	defer upsertFile.Close()

	getFileID, err := tx.Prepare("SELECT id FROM files WHERE path = ?")
	if err != nil {
		return 0, 0, 0, err
	}
	defer getFileID.Close()

	delSyms, err := tx.Prepare("DELETE FROM symbols WHERE file_id = ?")
	if err != nil {
		return 0, 0, 0, err
	}
	defer delSyms.Close()

	delEdges, err := tx.Prepare("DELETE FROM edges WHERE source_file_id = ?")
	if err != nil {
		return 0, 0, 0, err
	}
	defer delEdges.Close()

	insSym, err := tx.Prepare("INSERT INTO symbols (file_id, name, kind, line) VALUES (?, ?, ?, ?)")
	if err != nil {
		return 0, 0, 0, err
	}
	defer insSym.Close()

	insEdge, err := tx.Prepare("INSERT INTO edges (source_file_id, target_path, symbol_name, kind) VALUES (?, ?, ?, ?)")
	if err != nil {
		return 0, 0, 0, err
	}
	defer insEdge.Close()

	for _, op := range ops {
		if _, err := upsertFile.Exec(op.RelPath, op.Lang, op.ContentHash); err != nil {
			return 0, 0, 0, err
		}

		var fileID int64
		if err := getFileID.QueryRow(op.RelPath).Scan(&fileID); err != nil {
			return 0, 0, 0, err
		}

		if _, err := delSyms.Exec(fileID); err != nil {
			return 0, 0, 0, err
		}
		if _, err := delEdges.Exec(fileID); err != nil {
			return 0, 0, 0, err
		}

		for _, s := range op.Symbols {
			if _, err := insSym.Exec(fileID, s.Name, s.Kind, s.Line); err != nil {
				return 0, 0, 0, err
			}
			symbols++
		}

		for _, e := range op.Edges {
			if _, err := insEdge.Exec(fileID, e.TargetPath, e.SymbolName, e.Kind); err != nil {
				return 0, 0, 0, err
			}
			edges++
		}

		files++
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, 0, err
	}
	return files, symbols, edges, nil
}

func (d *DB) Stats() (files, symbols, edges int, err error) {
	err = d.conn.QueryRow("SELECT COUNT(*) FROM files").Scan(&files)
	if err != nil {
		return
	}
	err = d.conn.QueryRow("SELECT COUNT(*) FROM symbols").Scan(&symbols)
	if err != nil {
		return
	}
	err = d.conn.QueryRow("SELECT COUNT(*) FROM edges").Scan(&edges)
	return
}
