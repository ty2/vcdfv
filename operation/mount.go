package operation

import (
	"errors"
	"fmt"
	"github.com/ty2/vcdfv/config"
	"github.com/ty2/vcdfv/vcd"
	"github.com/ty2/vcdfv/vmdiskop"
	"strings"
	"time"
)

type Mount struct {
	MountDir  string
	Options   *Options
	VcdConfig *config.Vcdfv
}

func (operationMount *Mount) Exec() (*ExecResult, error) {
	var err error

	if operationMount.MountDir == "" {
		err = errors.New("mount dir is empty")
		return (&StatusFailure{Error: err}).Exec()
	}

	if operationMount.Options.PvOrVolumeName == "" {
		err = errors.New("disk name is empty")
		return (&StatusFailure{Error: err}).Exec()
	}

	if operationMount.Options.DiskInitialSize == "" {
		err = errors.New("disk initial size is empty")
		return (&StatusFailure{Error: err}).Exec()
	}

	// init VDC
	vdc, err := vcd.NewVdc(&vcd.VcdConfig{
		ApiEndpoint: operationMount.VcdConfig.VcdApiEndpoint,
		Insecure:    operationMount.VcdConfig.VcdInsecure,
		User:        operationMount.VcdConfig.VcdUser,
		Password:    operationMount.VcdConfig.VcdPassword,
		Org:         operationMount.VcdConfig.VcdOrg,
		Vdc:         operationMount.VcdConfig.VcdVdc,
	})
	if err != nil {
		return (&StatusFailure{Error: err}).Exec()
	}

	// find this VM in VDC
	vm, err := FindVm(vdc, operationMount.VcdConfig.VcdVdcVApp)
	if err != nil {
		return (&StatusFailure{Error: err}).Exec()
	}

	var diskForMount *vcd.VdcDisk
	// Find exists disk
	foundDisk, err := vdc.FindDiskByDiskName(operationMount.Options.PvOrVolumeName)
	if err != nil {
		if err.Error() != "not found" {
			return (&StatusFailure{Error: err}).Exec()
		}
	} else if foundDisk != nil {
		if foundDisk.AttachedVm != nil {
			// try to detach disk
			vm, err := FindVm(vdc, operationMount.VcdConfig.VcdVdcVApp)
			if err != nil {
				return (&StatusFailure{Error: err}).Exec()
			}

			if vm.Name == foundDisk.AttachedVm.Name {
				// TODO Detach disk in VM
			}

			err = vdc.DetachDisk(vm, foundDisk)
			if err != nil {
				err = errors.New(fmt.Sprintf("disk is attached to VM: %s and cannot detach disk %s from the VM", foundDisk.AttachedVm.Name, foundDisk.Name))
				return (&StatusFailure{Error: err}).Exec()
			}
		}

		diskForMount = foundDisk
	}

	// if no disk is found in VDC, create new disk
	if diskForMount == nil {
		size, err := SizeStringToByteUnit(operationMount.Options.DiskInitialSize)
		if err != nil {
			return (&StatusFailure{Error: err}).Exec()
		}

		diskForMount, err = vdc.CreateDisk(&vcd.VdcDisk{
			Name: operationMount.Options.PvOrVolumeName,
			Size: size,
		})

		if err != nil {
			return (&StatusFailure{Error: err}).Exec()
		}

		// find new disk to get new disk id
		diskForMount, err = vdc.FindDiskByDiskName(operationMount.Options.PvOrVolumeName)

		if err != nil {
			return (&StatusFailure{Error: err}).Exec()
		}
	}

	// get block devices info for compare later
	beforeBlockDevices, err := vmdiskop.BlockDevices()
	if err != nil {
		return (&StatusFailure{Error: err}).Exec()
	}

	// check disk is mounted
	for _, blockDevice := range beforeBlockDevices {
		if blockDevice.Label == operationMount.Options.PvOrVolumeName {
			err := errors.New("already mount")
			return (&StatusFailure{Error: err}).Exec()
		}
	}

	// attach disk
	if diskForMount.AttachedVm == nil {
		err = vdc.AttachDisk(vm, diskForMount)
		if err != nil {
			return (&StatusFailure{Error: err}).Exec()
		}
	}

	blockDevices, err := vmdiskop.BlockDevices()
	if err != nil {
		return (&StatusFailure{Error: err}).Exec()
	}

	// get all before block device names for compare
	beforeBlockDeviceNames := map[string]interface{}{}
	for _, blockDevice := range beforeBlockDevices {
		beforeBlockDeviceNames[blockDevice.Name] = nil
	}

	// find new mounted device
	var mountedBlockDevice *vmdiskop.BlockDevice
	for _, blockDevice := range blockDevices {
		if _, ok := beforeBlockDeviceNames[blockDevice.Name]; !ok {
			if mountedBlockDevice != nil {
				err := errors.New(fmt.Sprintf("multiple new device is found: %s, %s", mountedBlockDevice.Name, blockDevice.Name))
				return (&StatusFailure{Error: err}).Exec()
			}
			// new device here
			mountedBlockDevice = blockDevice
		}
	}

	// fail to mount
	if mountedBlockDevice == nil {
		err := errors.New("no new new device is found")
		return (&StatusFailure{Error: err}).Exec()
	}

	// if disk is not format then format it
	if !vmdiskop.IsFormatted(mountedBlockDevice) {
		if operationMount.Options.FsType != "ext4" {
			err := errors.New(fmt.Sprintf("only support file format ext4, got: %s", operationMount.Options.FsType))
			return (&StatusFailure{Error: err}).Exec()
		}

		// get disk uuid
		diskIdArr := strings.Split(diskForMount.Id, ":")
		uuid := diskIdArr[len(diskIdArr)-1]

		// format disk
		err := vmdiskop.FormatDeviceToExt4(mountedBlockDevice, operationMount.Options.PvOrVolumeName, uuid, time.Minute)
		if err != nil {
			return (&StatusFailure{Error: err}).Exec()
		}
	}

	// mount disk
	err = vmdiskop.Mount(mountedBlockDevice, operationMount.MountDir, operationMount.Options.FsType, operationMount.Options.Readwrite)
	if err != nil {
		err := errors.New("no new new device is found")
		return (&StatusFailure{Error: err}).Exec()
	}

	// output
	return (&StatusSuccess{JsonMessageStruct: struct {
		DiskId       string `json:"diskId"`
		DiskName     string `json:"diskName"`
		VmDeviceName string `json:"vmDeviceName"`
		MountPoint   string `json:"mountPoint"`
	}{
		DiskId:       diskForMount.Id,
		DiskName:     diskForMount.Name,
		VmDeviceName: mountedBlockDevice.Name,
		MountPoint:   operationMount.MountDir,
	}}).Exec()
}
