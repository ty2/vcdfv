package operation

import (
	"errors"
	"github.com/ty2/vcdfv/config"
	"github.com/ty2/vcdfv/vcd"
	"github.com/ty2/vcdfv/vmdiskop"
)

type Unmount struct {
	MountDir  string
	VcdConfig *config.Vcdfv
}

func (operationUnmount *Unmount) Exec() (*ExecResult, error) {
	if operationUnmount.MountDir == "" {
		err := errors.New("mount dir is empty")
		return (&StatusFailure{Error: err}).Exec()
	}

	// init vdc
	vdc, err := vcd.NewVdc(&vcd.VcdConfig{
		ApiEndpoint: operationUnmount.VcdConfig.VcdApiEndpoint,
		Insecure:    operationUnmount.VcdConfig.VcdInsecure,
		User:        operationUnmount.VcdConfig.VcdUser,
		Password:    operationUnmount.VcdConfig.VcdPassword,
		Org:         operationUnmount.VcdConfig.VcdOrg,
		Vdc:         operationUnmount.VcdConfig.VcdVdc,
	})
	if err != nil {
		return (&StatusFailure{Error: err}).Exec()
	}

	// find this VM is VDC
	vm, err := FindVm(vdc, operationUnmount.VcdConfig.VcdVdcVApp)
	if err != nil {
		return (&StatusFailure{Error: err}).Exec()
	}

	blockDevice, err := vmdiskop.FindDeviceByMountPoint(operationUnmount.MountDir)
	if err != nil {
		return (&StatusFailure{Error: err}).Exec()
	}

	var diskForUnmount *vcd.VdcDisk
	// Find exists disk
	foundDisk, err := vdc.FindDiskByDiskName(blockDevice.Label)
	if err != nil {
		return (&StatusFailure{Error: err}).Exec()
	} else {
		diskForUnmount = foundDisk
	}

	// unmount
	err = vmdiskop.Unmount(operationUnmount.MountDir)
	if err != nil {
		return (&StatusFailure{Error: err}).Exec()
	}

	// remove scsi device
	err = vmdiskop.RemoveSCSIDevice(blockDevice)
	if err != nil {
		return (&StatusFailure{Error: err}).Exec()
	}

	// detach disk in vdc
	err = vdc.DetachDisk(vm, diskForUnmount)
	if err != nil {
		return (&StatusFailure{Error: err}).Exec()
	}

	// output
	return (&StatusSuccess{JsonMessageStruct: struct {
		DiskId       string `json:"diskId"`
		DiskName     string `json:"diskName"`
		VmDeviceName string `json:"vmDeviceName"`
		MountPoint   string `json:"mountPoint"`
	}{
		DiskId:       foundDisk.Id,
		DiskName:     foundDisk.Name,
		VmDeviceName: blockDevice.Name,
		MountPoint:   blockDevice.MountPoint,
	}}).Exec()
}
