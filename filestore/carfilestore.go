package filestore

import (
	"path/filepath"
)

type carStore struct {
	baseDir string
}

// NewLocalCarStore creates a CAR file store mounted on a given
// local directory path
func NewLocalCarStore(baseDir string) (CarFileStore, error) {
	baseDir, err := checkIsDir(baseDir)
	if err != nil {
		return nil, err
	}
	return &carStore{baseDir: baseDir}, nil
}

func (r *carStore) Path(key string) string {
	return filepath.Join(r.baseDir, key)
}
