package filestore

import "os"

type fd struct {
	*os.File
	filename string
}

func newFile(filename Path) (File, error) {
	var err error
	result := fd{filename: string(filename)}
	result.File, err = os.OpenFile(result.filename, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (f fd) Path() Path {
	return Path(f.filename)
}

func (f fd) Size() int64 {
	info, err := os.Stat(f.filename)
	if err != nil {
		return -1
	}
	return info.Size()
}
