package vmdiskop

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os/exec"
	"syscall"
	"time"
)

type lsblkOutput struct {
	BlockDevices []*BlockDevice `json:"blockdevices"`
}

type BlockDevice struct {
	Name       string         `json:"name"`
	FsType     string         `json:"fstype"`
	Label      string         `json:"label"`
	Uuid       string         `json:"uuid"`
	Children   []*BlockDevice `json:"children"`
	MountPoint string         `json:"mountpoint"`
	Size       string         `json:"size"`
}

func FindDeviceByMountPoint(mountPoint string) (*BlockDevice, error) {
	blockDevices, err := BlockDevices()
	if err != nil {
		return nil, err
	}

	var foundedBlockDevice *BlockDevice
	for _, blockDevice := range blockDevices {
		if blockDevice.MountPoint == mountPoint {
			foundedBlockDevice = blockDevice
			break
		}
	}

	if foundedBlockDevice == nil {
		return nil, errors.New("not found")
	}

	return foundedBlockDevice, nil
}

func Mount(blockDevice *BlockDevice, mountPoint string, fsType string, readWrite string) error {
	flag := uintptr(0)
	if readWrite == "ro" {
		flag = syscall.O_RDONLY
	}
	return syscall.Mount(fmt.Sprintf("/dev/%s", blockDevice.Name), mountPoint, fsType, flag, "")
}

func Unmount(mountPoint string) error {
	return syscall.Unmount(mountPoint, 0)
}

func ScanScsiHost() error {
	scsiPath := "/sys/class/scsi_host/"
	files, err := ioutil.ReadDir(scsiPath)
	if err != nil {
		return err
	}

	for _, file := range files {
		name := scsiPath + file.Name() + "/scan"
		data := []byte("- - -")
		err := ioutil.WriteFile(name, data, 0666)
		if err != nil {
			return err
		}
	}

	// wait for scan complete
	time.Sleep(time.Second * 3)

	return nil
}

func RemoveSCSIDevice(blockDevice *BlockDevice) error {
	scsiRemovePath := fmt.Sprintf("/sys/block/%s/device/delete", blockDevice.Name)
	err := ioutil.WriteFile(scsiRemovePath, []byte("1"), 0666)
	if err != nil {
		return err
	}

	return nil
}

func IsFormatted(blockDevice *BlockDevice) bool {
	if blockDevice.FsType == "" && blockDevice.Children == nil {
		return false
	}

	return true
}

func BlockDevices() ([]*BlockDevice, error) {
	err := ScanScsiHost()
	if err != nil {
		return nil, err
	}

	lsblk := exec.Command("lsblk", "--json", "--fs", "-b", "-o", "NAME,FSTYPE,LABEL,UUID,MOUNTPOINT,SIZE")
	output, err := lsblk.Output()
	if err != nil {
		return nil, err
	}

	var lsblkOutputStruct *lsblkOutput
	err = json.Unmarshal(output, &lsblkOutputStruct)
	if err != nil {
		return nil, err
	}

	return lsblkOutputStruct.BlockDevices, nil
}

func FormatDeviceToExt4(dev *BlockDevice, label string, uuid string, timeout time.Duration) error {
	mkfsExt4 := exec.Command("mkfs.ext4", fmt.Sprintf("/dev/%s", dev.Name), "-L", label, "-U", uuid)
	err := mkfsExt4.Start()
	if err != nil {
		return err
	}

	mkfsExt4StdOut, err := mkfsExt4.StdoutPipe()

	mkfsExt4DoneCh := make(chan error)

	go func() {
		mkfsExt4DoneCh <- mkfsExt4.Wait()
	}()

	select {
	case <-time.After(timeout):
		processKillError := mkfsExt4.Process.Kill()
		mkfsExt4StdOutput, mkfsExt4StdOutputError := ioutil.ReadAll(mkfsExt4StdOut)
		return errors.New(
			fmt.Sprintf("timeout, stdOut: %s, stdOut error: %s, process kill error: %s",
				string(mkfsExt4StdOutput),
				mkfsExt4StdOutputError,
				processKillError.Error(),
			),
		)
	case err = <-mkfsExt4DoneCh:
		if err != nil {
			return err
		}
	}

	return nil
}
