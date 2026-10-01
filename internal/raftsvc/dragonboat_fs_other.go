//go:build !windows

package raftsvc

import "github.com/lni/vfs"

func dragonboatFS() vfs.FS {
	return nil
}
