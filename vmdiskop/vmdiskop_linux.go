//+build linux

package vmdiskop

import (
	"fmt"
	"syscall"
)

func Mount(blockDevice *BlockDevice, mountPoint string, fsType string, readWrite string) error {
	flag := uintptr(0)
	if readWrite == "ro" {
		flag = syscall.O_RDONLY
	}
	return syscall.Mount(fmt.Sprintf("/dev/%s", blockDevice.Name), mountPoint, fsType, flag, "")
}
