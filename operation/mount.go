package operation

import (
	"errors"
	"fmt"
	"github.com/ty2/vcdfv/config"
	"github.com/ty2/vcdfv/vcd"
	"github.com/ty2/vcdfv/vmdiskop"
	"regexp"
	"strings"
	"time"
)

type Mount struct {
	MountDir    string
	Options     *Options
	VcdfvConfig *config.Vcdfv
	vdc         *vcd.Vdc
}

func (mount *Mount) Exec() (*ExecResult, error) {
	var err error

	if mount.MountDir == "" {
		err = errors.New("mount dir is empty")
		return (&StatusFailure{Error: err}).Exec()
	}

	if mount.Options.PvOrVolumeName == "" {
		err = errors.New("disk name is empty")
		return (&StatusFailure{Error: err}).Exec()
	}

	if mount.Options.DiskInitialSize == "" {
		err = errors.New("disk initial size is empty")
		return (&StatusFailure{Error: err}).Exec()
	}

	// init VDC
	mount.vdc, err = vcd.NewVdc(&vcd.VcdConfig{
		ApiEndpoint: mount.VcdfvConfig.VcdApiEndpoint,
		Insecure:    mount.VcdfvConfig.VcdInsecure,
		User:        mount.VcdfvConfig.VcdUser,
		Password:    mount.VcdfvConfig.VcdPassword,
		Org:         mount.VcdfvConfig.VcdOrg,
		Vdc:         mount.VcdfvConfig.VcdVdc,
	})
	if err != nil {
		return (&StatusFailure{Error: errors.New("new vdc: " + err.Error())}).Exec()
	}

	// find this VM in VDC
	vm, err := FindVm(mount.vdc, mount.VcdfvConfig.VcdVdcVApp)
	if err != nil {
		return (&StatusFailure{Error: errors.New("find VM: " + err.Error())}).Exec()
	}

	var diskForMount *vcd.VdcDisk
	// Find exists disk
	foundDisk, err := mount.vdc.FindDiskByDiskName(mount.Options.PvOrVolumeName)
	if err != nil {
		if err.Error() != "not found" {
			return (&StatusFailure{Error: errors.New("find disk by disk name foundDisk: " + err.Error())}).Exec()
		}
	} else if foundDisk != nil {
		if foundDisk.AttachedVm != nil {
			// detach disk
			if err := mount.detachDisk(foundDisk, vm); err != nil {
				return (&StatusFailure{Error: errors.New("found disk, detach disk: " + err.Error())}).Exec()
			}
		}

		diskForMount = foundDisk
	}

	// if no disk is found in VDC, create new disk
	if diskForMount == nil {
		diskForMount, err = mount.createDisk()
		if err != nil {
			err = errors.New(fmt.Sprintf("create disk: %s", err))
			return (&StatusFailure{Error: err}).Exec()
		}
	}

	// get block devices info for compare later
	beforeMountBlockDevices, err := vmdiskop.BlockDevices()
	if err != nil {
		return (&StatusFailure{Error: errors.New("before block devices: " + err.Error())}).Exec()
	}
	// check disk is mounted
	for _, blockDevice := range beforeMountBlockDevices {
		if blockDevice.Label == mount.Options.PvOrVolumeName {
			err := errors.New("already mount")
			return (&StatusFailure{Error: err}).Exec()
		}
	}

	// attach disk
	err = mount.vdc.AttachDisk(vm, diskForMount)
	if err != nil {
		return (&StatusFailure{Error: errors.New("attach disk: " + err.Error())}).Exec()
	}

	// get block devices after attached
	blockDevices, err := vmdiskop.BlockDevices()
	if err != nil {
		return (&StatusFailure{Error: errors.New("list block devices: " + err.Error())}).Exec()
	}

	// found attached disk in block device list
	mountedBlockDevice, err := mount.findMountedDevice(beforeMountBlockDevices, blockDevices)
	if mountedBlockDevice == nil {
		// fail to mount
		err := errors.New("cannot found new device")
		return (&StatusFailure{Error: err}).Exec()
	} else if err != nil {
		err := errors.New("find mounted device: " + err.Error())
		return (&StatusFailure{Error: err}).Exec()
	}

	// if disk is not format then format it
	if !vmdiskop.IsFormatted(mountedBlockDevice) {
		if err = mount.formatDisk(diskForMount, mountedBlockDevice); err != nil {
			return (&StatusFailure{Error: errors.New("format disk error:" + err.Error())}).Exec()
		}
	}

	// set disk meta
	err = mount.setDiskMeta(diskForMount, mountedBlockDevice, vm)
	if err != nil {
		err := errors.New("set disk meta: " + err.Error())
		return (&StatusFailure{Error: err}).Exec()
	}

	// mount disk
	err = vmdiskop.Mount(mountedBlockDevice, mount.MountDir, mount.Options.FsType, mount.Options.Readwrite)
	if err != nil {
		return (&StatusFailure{Error: errors.New("mount: " + err.Error())}).Exec()
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
		MountPoint:   mount.MountDir,
	}}).Exec()
}

func (mount *Mount) findMountedDevice(beforeMountedBlockDevices []*vmdiskop.BlockDevice, afterMountedBlockDevices []*vmdiskop.BlockDevice) (*vmdiskop.BlockDevice, error) {
	// get all before block device names for compare
	beforeBlockDeviceNames := map[string]interface{}{}
	for _, blockDevice := range beforeMountedBlockDevices {
		beforeBlockDeviceNames[blockDevice.Name] = nil
	}

	// find new mounted device
	var mountedBlockDevice *vmdiskop.BlockDevice
	for _, blockDevice := range afterMountedBlockDevices {
		if _, ok := beforeBlockDeviceNames[blockDevice.Name]; !ok {
			if mountedBlockDevice != nil {
				return nil, errors.New(fmt.Sprintf("multiple new device is found: %s, %s", mountedBlockDevice.Name, blockDevice.Name))
			}
			// new device here
			mountedBlockDevice = blockDevice
		}
	}

	return mountedBlockDevice, nil
}

func (mount *Mount) createDisk() (*vcd.VdcDisk, error) {
	size, err := SizeStringToByteUnit(mount.Options.DiskInitialSize)
	if err != nil {
		return nil, errors.New("size string to byte unit: " + err.Error())
	}

	disk, err := mount.vdc.CreateDisk(&vcd.VdcDisk{
		Name: mount.Options.PvOrVolumeName,
		Size: size,
	})

	if err != nil {
		return nil, errors.New("create disk: " + err.Error())
	}

	// find new disk to get new disk id
	disk, err = mount.vdc.FindDiskByDiskName(mount.Options.PvOrVolumeName)
	if err != nil {
		return nil, errors.New("find disk by disk name diskForMount: " + err.Error())
	}

	return disk, nil
}

func (mount *Mount) formatDisk(disk *vcd.VdcDisk, blockDevice *vmdiskop.BlockDevice) error {
	if mount.Options.FsType != "ext4" {
		return errors.New(fmt.Sprintf("only support file format ext4, got: %s", mount.Options.FsType))
	}

	// get disk UUID
	diskIdArr := strings.Split(disk.Id, ":")
	uuid := diskIdArr[len(diskIdArr)-1]

	// check whether UUID is valid
	if !regexp.MustCompile("^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-4[a-fA-F0-9]{3}-[8|9|aA|bB][a-fA-F0-9]{3}-[a-fA-F0-9]{12}$").MatchString(uuid) {
		return errors.New("format device to ext4 UUID is invalid: " + disk.Id)
	}

	// format disk
	output, err := vmdiskop.FormatDeviceToExt4(blockDevice, disk.Name, uuid, time.Minute)
	if err != nil {
		return errors.New(fmt.Sprintf("format device to ext4: %s, %s", err.Error(), output))
	}

	return nil
}

func (mount *Mount) setDiskMeta(disk *vcd.VdcDisk, blockDevice *vmdiskop.BlockDevice, vm *vcd.VAppVm) error {
	// set disk meta
	// 1. must detach disk before update disk info
	// 2. update disk info
	// 3. attach disk back
	// 4. refresh blk list
	err := mount.vdc.DetachDisk(vm, disk)
	if err != nil {
		return errors.New(fmt.Sprintf("detach disk: %s", err.Error()))
	}

	disk, err = mount.vdc.SetDiskMeta(disk, &vcd.VdcDiskMeta{
		VmName:     vm.Name,
		DeviceName: blockDevice.Name,
	})
	if err != nil {
		return errors.New(fmt.Sprintf("%s", err.Error()))
	}

	err = vmdiskop.RemoveSCSIDevice(blockDevice)
	if err != nil {
		return errors.New(fmt.Sprintf("sremove SCSI Device: %s", err.Error()))
	}

	blockDevices, err := vmdiskop.BlockDevices()
	if err != nil {
		return errors.New(fmt.Sprintf("list block devices: %s", err.Error()))
	}

	err = mount.vdc.AttachDisk(vm, disk)
	if err != nil {
		return errors.New(fmt.Sprintf("attach disk: %s", err.Error()))
	}

	afterScannedBlockDevices, err := vmdiskop.BlockDevices()
	if err != nil {
		return errors.New(fmt.Sprintf("list block devices 2: %s", err.Error()))
	}

	afterScannedBlockDevice, err := mount.findMountedDevice(blockDevices, afterScannedBlockDevices)
	if err != nil {
		// not found or other error
		return errors.New(fmt.Sprintf("after scanned block device: %s", err.Error()))
	}

	// assume disk is formatted
	// make sure before detach and after attach disk is the same
	if afterScannedBlockDevice.Label != blockDevice.Label {
		// not the same, set disk meta again
		return mount.setDiskMeta(disk, afterScannedBlockDevice, vm)
	}

	return nil
}

func (mount *Mount) detachDisk(disk *vcd.VdcDisk, vm *vcd.VAppVm) error {
	if vm.Name == disk.AttachedVm.Name {
		// TODO Detach disk in VM by HCTL
		if disk.Meta != nil {
			vmdiskop.RemoveSCSIDevice(&vmdiskop.BlockDevice{
				Name: disk.Meta.DeviceName,
			})
		}
	}

	err := mount.vdc.DetachDisk(vm, disk)
	if err != nil {
		return errors.New(fmt.Sprintf("disk is attached to VM %s and cannot detach disk %s from the VM", disk.AttachedVm.Name, disk.Name))
	}

	return nil
}
