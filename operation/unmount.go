package operation

import (
	"errors"
	"fmt"
	"github.com/ty2/vcdfv/config"
	"github.com/ty2/vcdfv/vcd"
	"github.com/ty2/vcdfv/vmdiskop"
	"strings"
)

type Unmount struct {
	MountDir    string
	VcdfvConfig *config.Vcdfv
	vdc         *vcd.Vdc
}

func (unmount *Unmount) Exec() (*ExecResult, error) {
	var err error

	// when manualUnmount is true, ignore unmount process and return success
	if unmount.VcdfvConfig.ManualUnmount {
		return (&StatusSuccess{JsonMessageStruct: struct {
			DiskId       string `json:"diskId"`
			DiskName     string `json:"diskName"`
			VmDeviceName string `json:"vmDeviceName"`
			MountPoint   string `json:"mountPoint"`
		}{
			DiskName:   "<manual-unmount>",
			MountPoint: unmount.MountDir,
		}}).Exec()
	}

	if unmount.MountDir == "" {
		err := errors.New("mount dir is empty")
		return (&StatusFailure{Error: err}).Exec()
	}

	// init vdc
	unmount.vdc, err = VdcClient(unmount.VcdfvConfig)
	if err != nil {
		return (&StatusFailure{Error: errors.New("vdc client: " + err.Error())}).Exec()
	}

	// find this VM is VDC
	vm, err := FindVm(unmount.vdc, unmount.VcdfvConfig.VcdVdcVApp)
	if err != nil {
		return (&StatusFailure{Error: errors.New("find VM: " + err.Error())}).Exec()
	}

	// find block device
	var diskForUnmount *vcd.VdcDisk
	blockDeviceForUnmount, err := vmdiskop.FindDeviceByMountPoint(unmount.MountDir)
	if err != nil {
		diskForUnmount, err = unmount.findAttachedDiskByBlockDeviceInfo(blockDeviceForUnmount)
		if err != nil {
			return (&StatusFailure{Error: errors.New("find attached disk by block device info: " + err.Error())}).Exec()
		}
	} else {
		mountPointErr := err
		// block device not found or error, then use disk meta data and mount dir pv name for find device
		blockDeviceForUnmount, diskForUnmount, err = unmount.findAttachedDiskAndBlockDeviceByMountDirAndVm(vm)
		if err != nil {
			return (&StatusFailure{Error: errors.New(fmt.Sprintf("find attach disk and block device by mount dir and vm: %s,%s", err.Error(), mountPointErr.Error()))}).Exec()
		}
	}

	// unmount (ignore error because if disk was unmounted, it will return error)
	vmdiskop.Unmount(unmount.MountDir)

	// remove scsi device
	err = vmdiskop.RemoveSCSIDevice(blockDeviceForUnmount)
	if err != nil {
		return (&StatusFailure{Error: errors.New("remove SCSI device: " + err.Error())}).Exec()
	}

	// detach disk in vdc
	err = unmount.vdc.DetachDisk(vm, diskForUnmount)
	if err != nil {
		return (&StatusFailure{Error: errors.New("detach disk: " + err.Error())}).Exec()
	}

	// reset disk meta
	diskForUnmount, err = unmount.vdc.SetDiskMeta(diskForUnmount, &vcd.VdcDiskMeta{
		VmName:     "",
		DeviceName: "",
	})
	if err != nil {
		return (&StatusFailure{Error: errors.New("set disk meta: " + err.Error())}).Exec()
	}

	// output
	return (&StatusSuccess{JsonMessageStruct: struct {
		DiskId       string `json:"diskId"`
		DiskName     string `json:"diskName"`
		VmDeviceName string `json:"vmDeviceName"`
		MountPoint   string `json:"mountPoint"`
	}{
		DiskId:       diskForUnmount.Id,
		DiskName:     diskForUnmount.Name,
		VmDeviceName: blockDeviceForUnmount.Name,
		MountPoint:   blockDeviceForUnmount.MountPoint,
	}}).Exec()
}

func (unmount *Unmount) findAttachedDiskAndBlockDeviceByMountDirAndVm(vm *vcd.VAppVm) (*vmdiskop.BlockDevice, *vcd.VdcDisk, error) {
	// split mount dir for getting path segment
	mountDirArr := strings.Split(unmount.MountDir, "/")

	// the last segment of mount dir path is pv or volume name and assume pv or volume name is disk name
	diskName := mountDirArr[len(mountDirArr)-1]

	// find attached disk
	foundDisk, err := unmount.vdc.FindDiskByDiskName(diskName)
	if err != nil {
		return nil, nil, errors.New("find device by mount point, find disk by disk name error: " + err.Error())
	}

	// disk is not attached to this VM
	if foundDisk.AttachedVm != nil && foundDisk.AttachedVm.Name != vm.Name {
		return nil, nil, errors.New(fmt.Sprintf("find device by mount point, disk is not attached to this vm, expect: %s, got: %s: ", foundDisk.AttachedVm.Name, vm.Name))
	} else {
		return nil, nil, errors.New("disk is not attached to any VM")
	}

	// get meta data
	meta, err := unmount.vdc.DiskMeta(foundDisk)
	if err != nil {
		return nil, nil, errors.New("find device by mount point, get disk meta error: " + err.Error())
	}

	// find and set block device
	blockDevice, err := vmdiskop.FindDeviceByDeviceName(meta.DeviceName)
	if err != nil {
		return nil, nil, errors.New(fmt.Sprintf("find device by mount point, find device by device name error: %s", err.Error()))
	}

	return blockDevice, foundDisk, nil
}

func (unmount *Unmount) findAttachedDiskByBlockDeviceInfo(blockDeviceInfo *vmdiskop.BlockDevice) (*vcd.VdcDisk, error) {
	// find exists disk
	foundDisk, err := unmount.vdc.FindDiskByDiskName(blockDeviceInfo.Label)
	if err != nil {
		return nil, errors.New("find disk by disk name: " + err.Error())
	}

	return foundDisk, nil
}
