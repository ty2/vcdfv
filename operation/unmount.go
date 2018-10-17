package operation

import (
	"encoding/json"
	"errors"
	"github.com/ty2/vcdfv/vcd"
	"github.com/ty2/vcdfv/vmdiskop"
)

type Unmount struct {
	MountDir  string
	VcdConfig *VcdConfig
}

func (operationUnmount *Unmount) Exec() (*ExecResult, error) {
	if operationUnmount.MountDir == "" {
		err := errors.New("mount dir is empty")
		return (&StatusFailure{Error: err}).Exec()
	}

	// init vdc
	vdc, err := vcd.NewVdc(&vcd.VcdConfig{
		ApiEndpoint: operationUnmount.VcdConfig.ApiEndpoint,
		Insecure:    operationUnmount.VcdConfig.Insecure,
		User:        operationUnmount.VcdConfig.User,
		Password:    operationUnmount.VcdConfig.Password,
		Org:         operationUnmount.VcdConfig.Org,
		Vdc:         operationUnmount.VcdConfig.Vdc,
	})
	if err != nil {
		return (&StatusFailure{Error: err}).Exec()
	}

	// find this VM is VDC
	vm, err := FindVm(vdc, operationUnmount.VcdConfig.VdcVApp)
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
	message, err := json.Marshal(struct {
		DiskId       string `json:"diskId"`
		DiskName     string `json:"diskName"`
		VmDeviceName string `json:"vmDeviceName"`
		MountPoint   string `json:"mountPoint"`
	}{
		DiskId:       foundDisk.Id,
		DiskName:     foundDisk.Name,
		VmDeviceName: blockDevice.Name,
		MountPoint:   blockDevice.MountPoint,
	})
	if err != nil {
		return (&StatusFailure{Error: err}).Exec()
	}
	return &ExecResult{
		Status:  ExecResultStatusSuccess,
		Message: string(message),
	}, nil
}
