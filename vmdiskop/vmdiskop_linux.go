//+build linux

package vmdiskop

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
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
	if blockDevice.FsType == "" && len(blockDevice.Children) <= 0 {
		return false
	}

	return true
}

func BlockDevices() ([]*BlockDevice, error) {
	err := ScanScsiHost()
	if err != nil {
		return nil, err
	}

	// Note that lsblk might be executed in time when udev does not have all
	// information about recently added or modified devices yet. In this
	// case it is recommended to use udevadm settle before lsblk to
	// synchronize with udev.
	// http://man7.org/linux/man-pages/man8/lsblk.8.html
	udevadm := exec.Command("udevadm", "settle")
	err = udevadm.Run()
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
		return nil, errors.New(fmt.Sprintf("json unmarshal err: %s, %s", err.Error(), string(output)))
	}

	return lsblkOutputStruct.BlockDevices, nil
}

func FormatDeviceToExt4(dev *BlockDevice, label string, uuid string, timeout time.Duration) (string, error) {
	mkfsExt4 := exec.Command("mkfs.ext4", fmt.Sprintf("/dev/%s", dev.Name), "-L", label, "-U", uuid)
	stdoutReader, stdout, _ := os.Pipe()
	mkfsExt4.Stdout = stdout

	stderrReader, stderr, _ := os.Pipe()
	mkfsExt4.Stderr = stderr
	cmdReader := io.MultiReader(stdoutReader, stderrReader)

	// execute
	mkfsExt4DoneCh := make(chan error)
	go func() {
		defer stdout.Close()
		defer stderr.Close()
		mkfsExt4DoneCh <- mkfsExt4.Run()
	}()

	select {
	case <-time.After(timeout):
		processKillError := mkfsExt4.Process.Kill()
		mkfsExt4Output, mkfsExt4OutputError := ioutil.ReadAll(cmdReader)
		return string(mkfsExt4Output), errors.New(
			fmt.Sprintf("timeout, stdout: %s, output error: %v, process kill error: %v",
				string(mkfsExt4Output),
				mkfsExt4OutputError,
				processKillError,
			),
		)
	case err := <-mkfsExt4DoneCh:
		// return output and result error
		mkfsExt4Output, mkfsExt4OutputError := ioutil.ReadAll(cmdReader)
		output := ""
		if mkfsExt4OutputError != nil {
			output = mkfsExt4OutputError.Error()
		} else {
			output = string(mkfsExt4Output)
		}
		return output, err
	}

	return "", nil
}
