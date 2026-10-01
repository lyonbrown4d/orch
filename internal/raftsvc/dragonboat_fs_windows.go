//go:build windows

package raftsvc

import (
	"errors"
	"fmt"

	"github.com/lni/vfs"
)

type windowsTanFS struct {
	vfs.FS
}

type windowsDirectoryFile struct {
	vfs.File
}

func (windowsDirectoryFile) Sync() error {
	return nil
}

func (fs windowsTanFS) Open(name string, opts ...vfs.OpenOption) (vfs.File, error) {
	file, err := fs.FS.Open(name, opts...)
	if err != nil {
		return nil, fmt.Errorf("open Dragonboat path %q: %w", name, err)
	}
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat Dragonboat path %q: %w", name, errors.Join(err, file.Close()))
	}
	if info.IsDir() {
		return windowsDirectoryFile{File: file}, nil
	}
	return file, nil
}

func (fs windowsTanFS) OpenDir(name string) (vfs.File, error) {
	file, err := fs.FS.OpenDir(name)
	if err != nil {
		return nil, fmt.Errorf("open Dragonboat directory %q: %w", name, err)
	}
	return windowsDirectoryFile{File: file}, nil
}

func dragonboatFS() vfs.FS {
	return windowsTanFS{FS: vfs.Default}
}
