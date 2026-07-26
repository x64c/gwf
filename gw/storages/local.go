package storages

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrPathEscapesRoot is returned when a requested path resolves outside the
// storage's configured root directory (a path-traversal attempt). In-root
// navigation — including ".." segments that stay within root — is allowed;
// only paths resolving outside root are rejected.
var ErrPathEscapesRoot = errors.New("storage: path escapes root")

// LocalStorage implements Storage for local disk. All access is confined to
// the configured root; see ErrPathEscapesRoot. The confinement is lexical
// (filepath.Clean-based) — it defends against ".." traversal but does not
// resolve symlinks. The storage API only ever creates regular files, so a
// symlink can't be planted through it.
type LocalStorage struct {
	root string
}

func NewLocalStorage(root string) *LocalStorage {
	return &LocalStorage{root: filepath.Clean(root)}
}

// path resolves p against root and rejects any result that escapes root.
func (s *LocalStorage) path(p string) (string, error) {
	full := filepath.Join(s.root, p)
	if full != s.root && !strings.HasPrefix(full, s.root+string(os.PathSeparator)) {
		return "", ErrPathEscapesRoot
	}
	return full, nil
}

func (s *LocalStorage) Exists(_ context.Context, path string) (bool, error) {
	full, err := s.path(path)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(full)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (s *LocalStorage) Get(_ context.Context, path string) (io.ReadCloser, error) {
	full, err := s.path(path)
	if err != nil {
		return nil, err
	}
	return os.Open(full)
}

func (s *LocalStorage) Put(_ context.Context, path string, r io.Reader) error {
	full, err := s.path(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		return err
	}
	f, err := os.Create(full)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(f, r)
	return err
}

func (s *LocalStorage) Delete(_ context.Context, path string) error {
	full, err := s.path(path)
	if err != nil {
		return err
	}
	return os.Remove(full)
}

func (s *LocalStorage) Size(_ context.Context, path string) (int64, error) {
	full, err := s.path(path)
	if err != nil {
		return 0, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func (s *LocalStorage) Copy(_ context.Context, src string, dst string) error {
	srcFull, err := s.path(src)
	if err != nil {
		return err
	}
	dstFull, err := s.path(dst)
	if err != nil {
		return err
	}
	srcFile, err := os.Open(srcFull)
	if err != nil {
		return err
	}
	defer func() { _ = srcFile.Close() }()
	if err = os.MkdirAll(filepath.Dir(dstFull), 0755); err != nil {
		return err
	}
	dstFile, err := os.Create(dstFull)
	if err != nil {
		return err
	}
	defer func() { _ = dstFile.Close() }()
	_, err = io.Copy(dstFile, srcFile)
	return err
}

func (s *LocalStorage) Move(_ context.Context, src string, dst string) error {
	srcFull, err := s.path(src)
	if err != nil {
		return err
	}
	dstFull, err := s.path(dst)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dstFull), 0755); err != nil {
		return err
	}
	return os.Rename(srcFull, dstFull)
}
