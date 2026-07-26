package mysql

import (
	"fmt"
	"io/fs"
	"log"
	"path/filepath"
	"strings"

	"github.com/x64c/gwf/gw/sqldbs"
)

func loadRawStmtsToStore(store *sqldbs.RawSQLStore, sqlFS fs.FS) error {
	stmtCnt := 0
	err := fs.WalkDir(sqlFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// error reading a directory
			return err
		}
		if d.IsDir() {
			// Skip directory itself. still walking into it.
			return nil
		}
		ext := filepath.Ext(path) // with the leading dot
		if ext == "" {
			return nil
		}
		plainExt := strings.TrimPrefix(ext, ".")
		if plainExt != "mysql" && plainExt != "sql" {
			return nil
		}
		data, err := fs.ReadFile(sqlFS, path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
		}
		// Build key ("foo/bar/find" → "foo.bar.find")
		// fs.FS always stores paths using forward slashes '/', regardless of the OS.
		key := strings.TrimSuffix(path, ext)
		key = strings.TrimPrefix(key, "./") // just in case that fs is not an embed.fs
		key = strings.ReplaceAll(key, "/", ".")

		if plainExt == "mysql" {
			// exact matching file extension -> use it as-is for dialects
			store.Set(key, string(data))
			stmtCnt++
			return nil
		}
		// .sql (standard) fallback — only if no dialect-specific file loaded
		if _, exists := store.Get(key); !exists {
			// Placeholders: `?` (static) and `??` (dynamic) -> No conversion needed
			store.Set(key, string(data))
			stmtCnt++
		}
		return nil
	})
	if err != nil {
		return err
	}
	log.Printf("[INFO][mysql] %d sql raw stmts loaded", stmtCnt)
	return nil
}
