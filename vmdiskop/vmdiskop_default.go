//+build !linux

package vmdiskop

import "errors"

func Mount(blockDevice *BlockDevice, mountPoint string, fsType string, readWrite string) error {
	return errors.New("not support")
}
