package filestore_test

import (
	"crypto/rand"
	"fmt"
	"os"
	"testing"

	filestore "github.com/filecoin-project/go-fil-filestore"
	"github.com/stretchr/testify/require"
)

func randBytes(n int) []byte {
	arr := make([]byte, n)
	rand.Read(arr)
	return arr
}

const baseDir = "_test/a/b/c/d"
const existingFile = "existing.txt"

func init() {
	os.MkdirAll(baseDir, 0755)
	filename := fmt.Sprintf("%s%c%s", baseDir, os.PathSeparator, existingFile)
	file, err := os.Create(filename)
	if err != nil {
		return
	}
	defer file.Close()
	file.Write(randBytes(64))
}

func Test_SizeFails(t *testing.T) {
	store, err := filestore.NewLocalFileStore(baseDir)
	require.NoError(t, err)
	name := filestore.Path("newFile.txt")
	file, err := store.Create(name)
	require.NoError(t, err)
	err = store.Delete(name)
	require.NoError(t, err)
	require.Equal(t, int64(-1), file.Size())
}

func Test_OpenFileFails(t *testing.T) {
	base := "_test/a/b/c/d/e"
	err := os.MkdirAll(base, 0755)
	require.NoError(t, err)
	store, err := filestore.NewLocalFileStore(base)
	require.NoError(t, err)
	err = os.Remove(base)
	require.NoError(t, err)
	_, err = store.Open(existingFile)
	require.Error(t, err)
}

func Test_RemoveSeparators(t *testing.T) {
	first, err := filestore.NewLocalFileStore(baseDir)
	require.NoError(t, err)
	second, err := filestore.NewLocalFileStore(fmt.Sprintf("%s%c%c", baseDir, os.PathSeparator, os.PathSeparator))
	require.NoError(t, err)
	f1, err := first.Open(existingFile)
	require.NoError(t, err)
	f2, err := second.Open(existingFile)
	require.NoError(t, err)
	require.Equal(t, f1.Path(), f2.Path())
}

func Test_BaseDirIsFileFails(t *testing.T) {
	base := fmt.Sprintf("%s%c%s", baseDir, os.PathSeparator, existingFile)
	_, err := filestore.NewLocalFileStore(base)
	require.Error(t, err)
}

func Test_CreateExistingFileFails(t *testing.T) {
	store, err := filestore.NewLocalFileStore(baseDir)
	require.NoError(t, err)
	_, err = store.Create(filestore.Path(existingFile))
	require.Error(t, err)
}

func Test_StoreFails(t *testing.T) {
	store, err := filestore.NewLocalFileStore(baseDir)
	require.NoError(t, err)
	file, err := store.Open(filestore.Path(existingFile))
	require.NoError(t, err)
	err = store.Store(filestore.Path(existingFile), file)
	require.Error(t, err)
}

func Test_OpenFails(t *testing.T) {
	store, err := filestore.NewLocalFileStore(baseDir)
	require.NoError(t, err)
	name := filestore.Path("newFile.txt")
	_, err = store.Open(name)
	require.Error(t, err)
}

func Test_InvalidBaseDirectory(t *testing.T) {
	_, err := filestore.NewLocalFileStore("NoSuchDirectory")
	require.Error(t, err)
}

func Test_CreateFile(t *testing.T) {
	store, err := filestore.NewLocalFileStore(baseDir)
	require.NoError(t, err)
	name := filestore.Path("newFile.txt")
	path := fmt.Sprintf("%s%c%s", baseDir, os.PathSeparator, name)
	file, err := store.Create(name)
	require.NoError(t, err)
	defer func () { store.Delete(name) } ()
	require.Equal(t, filestore.Path(path), file.Path())
	bytesToWrite := 32
	written, err := file.Write(randBytes(bytesToWrite))
	require.NoError(t, err)
	require.Equal(t, bytesToWrite, written)
	require.Equal(t, int64(bytesToWrite), file.Size())
}

func Test_OpenAndReadFile(t *testing.T) {
	store, err := filestore.NewLocalFileStore(baseDir)
	require.NoError(t, err)
	file, err := store.Open(filestore.Path(existingFile))
	require.NoError(t, err)
	size := file.Size()
	require.NotEqual(t, -1, size)
	pos := int64(size / 2)
	offset, err := file.Seek(pos, 0)
	require.NoError(t, err)
	require.Equal(t, pos, offset)
	buffer := make([]byte, size / 2)
	read, err := file.Read(buffer)
	require.NoError(t, err)
	require.Equal(t, int(size / 2), read)
	err = file.Close()
	require.NoError(t, err)
}

func Test_CopyFile(t *testing.T) {
	store, err := filestore.NewLocalFileStore(baseDir)
	require.NoError(t, err)
	file, err := store.Open(filestore.Path(existingFile))
	require.NoError(t, err)
	newFile := filestore.Path("newFile.txt")
	err = store.Store(newFile, file)
	require.NoError(t, err)
	err = store.Delete(newFile)
	require.NoError(t, err)
}
