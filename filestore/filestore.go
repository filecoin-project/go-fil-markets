package filestore

import (
	"fmt"
	"io"
	"os"
)

type fileStore struct {
	base string
}

// NewLocalFileStore creates a filestore mounted on a given local directory path
func NewLocalFileStore(basedirectory string) (FileStore, error) {
	i := len(basedirectory) - 1
	for ; i >= 0; i-- {
		if basedirectory[i] != os.PathSeparator {
			break
		}
	}
	base := basedirectory[0:i + 1]
	info, err := os.Stat(base)
	if err != nil {
		return nil, fmt.Errorf("error getting %s info: %s", base, err.Error())
	}
	if false == info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", base)
	}
	return &fileStore{base}, nil
}

func (fs fileStore) filename(p Path) string {
	return fmt.Sprintf("%s%c%s", fs.base, os.PathSeparator, p)
}

func (fs fileStore) Open(p Path) (File, error) {
	name := fs.filename(p)
	_, err := os.Stat(name)
	if err != nil {
		return nil, fmt.Errorf("error trying to open %s: %s", name, err.Error())
	}
	return newFile(Path(name))
}

func (fs fileStore) Create(p Path) (File, error) {
	name := fs.filename(p)
	_, err := os.Stat(name)
	if err == nil {
		return nil, fmt.Errorf("file %s already exists", name)
	}
	return newFile(Path(name))
}

func (fs fileStore) Store(p Path, src File) error {
	dest, err := fs.Create(p)
	if err != nil {
		return err
	}
	defer dest.Close()
	_, err = io.Copy(dest, src)
	return err
}

func (fs fileStore) Delete(p Path) error {
	return os.Remove(fs.filename(p))
}


