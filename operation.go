// Created by terry on 11/10/2018.

package vcdfv

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ty2/vcdfv/vmdiskop"
	"strings"
	"time"
)

type Operation interface {
	Exec() (*OperationExecResult, error)
}

/*
{
	"status": "<Success/Failure/Not supported>",
	"message": "<Reason for success/failure>",
	"device": "<Path to the device attached. This field is valid only for attach & waitforattach call-outs>"
	"volumeName": "<Cluster wide unique name of the volume. Valid only for getvolumename call-out>"
	"attached": <True/False (Return true if volume is attached on the node. Valid only for isattached call-out)>
    "capabilities": <Only included as part of the Init response>
    {
        "attach": <True/False (Return true if the driver implements attach and detach)>
    }
}
*/

const (
	OperationExecResultStatusSuccess      = "Success"
	OperationExecResultStatusFailure      = "Failure"
	OperationExecResultStatusNotSupported = "Not supported"
)

type OperationVcdConfig struct {
	ApiEndpoint string `yaml:"apiEndpoint"`
	Insecure    bool   `yaml:"insecure"`
	User        string `yaml:"user"`
	Password    string `yaml:"password"`
	Org         string `yaml:"org"`
	Vdc         string `yaml:"vdc"`
	VdcVApp     string `yaml:"vdcVApp"`
}

type OperationExecResult struct {
	Status       string                     `json:"status"`
	Message      string                     `json:"message"`
	Capabilities *OperationExecCapabilities `json:"capabilities,omitempty"`
}

type OperationExecCapabilities struct {
	Attach bool `json:"attach"`
}

type OperationOptions struct {
	FsType         string `json:"kubernetes.io/fsType"`
	Readwrite      string `json:"kubernetes.io/readwrite"`
	FsGroup        string `json:"kubernetes.io/fsGroup"`
	PvOrVolumeName string `json:"kubernetes.io/pvOrVolumeName"`
	// Addition Options
	DiskInitialSize string `json:"diskInitialSize"`
}

type OperationInit struct {
	Operation
}

func (operationInit *OperationInit) Exec() (*OperationExecResult, error) {
	return &OperationExecResult{
		Status:  OperationExecResultStatusSuccess,
		Message: "Initial success",
		Capabilities: &OperationExecCapabilities{
			Attach: false,
		},
	}, nil
}

type OperationMount struct {
	Operation
	MountDir  string
	Options   *OperationOptions
	VcdConfig *OperationVcdConfig
}

func (operationMount *OperationMount) Exec() (*OperationExecResult, error) {
	var err error

	if operationMount.MountDir == "" {
		err = errors.New("mount dir is empty")
		return (&OperationStatusFailure{Error: err}).Exec()
	}

	if operationMount.Options.PvOrVolumeName == "" {
		err = errors.New("disk name is empty")
		return (&OperationStatusFailure{Error: err}).Exec()
	}

	if operationMount.Options.DiskInitialSize == "" {
		err = errors.New("disk initial size is empty")
		return (&OperationStatusFailure{Error: err}).Exec()
	}

	// init VDC
	vdc, err := NewVdc(&VcdConfig{
		ApiEndpoint: operationMount.VcdConfig.ApiEndpoint,
		Insecure:    operationMount.VcdConfig.Insecure,
		User:        operationMount.VcdConfig.User,
		Password:    operationMount.VcdConfig.Password,
		Org:         operationMount.VcdConfig.Org,
		Vdc:         operationMount.VcdConfig.Vdc,
	})
	if err != nil {
		return (&OperationStatusFailure{Error: err}).Exec()
	}

	// find this VM in VDC
	vm, err := FindVm(vdc, operationMount.VcdConfig.VdcVApp)
	if err != nil {
		return (&OperationStatusFailure{Error: err}).Exec()
	}

	var diskForMount *VdcDisk
	// Find exists disk
	foundDisk, err := vdc.FindDiskByDiskName(operationMount.Options.PvOrVolumeName)
	if err != nil {
		if err.Error() != "not found" {
			return (&OperationStatusFailure{Error: err}).Exec()
		}
	} else if foundDisk != nil {
		if foundDisk.AttachedVm != nil {
			err = errors.New(fmt.Sprintf("disk is attached to VM: %s", foundDisk.AttachedVm.Name))
			return (&OperationStatusFailure{Error: err}).Exec()
		}

		diskForMount = foundDisk
	}

	// if no disk is found in VDC, create new disk
	if diskForMount == nil {
		size, err := SizeStringToByteUnit(operationMount.Options.DiskInitialSize)
		if err != nil {
			return (&OperationStatusFailure{Error: err}).Exec()
		}

		diskForMount, err = vdc.CreateDisk(&VdcDisk{
			Name: operationMount.Options.PvOrVolumeName,
			Size: size,
		})

		if err != nil {
			return (&OperationStatusFailure{Error: err}).Exec()
		}

		// find new disk to get new disk id
		diskForMount, err = vdc.FindDiskByDiskName(operationMount.Options.PvOrVolumeName)

		if err != nil {
			return (&OperationStatusFailure{Error: err}).Exec()
		}
	}

	// get block devices info for compare later
	beforeBlockDevices, err := vmdiskop.BlockDevices()
	if err != nil {
		return (&OperationStatusFailure{Error: err}).Exec()
	}

	// check disk is mounted
	for _, blockDevice := range beforeBlockDevices {
		if blockDevice.Label == operationMount.Options.PvOrVolumeName {
			err := errors.New("already mount")
			return (&OperationStatusFailure{Error: err}).Exec()
		}
	}

	// attach disk
	if diskForMount.AttachedVm == nil {
		err = vdc.AttachDisk(vm, diskForMount)
		if err != nil {
			return (&OperationStatusFailure{Error: err}).Exec()
		}
	}

	blockDevices, err := vmdiskop.BlockDevices()
	if err != nil {
		return (&OperationStatusFailure{Error: err}).Exec()
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
				return (&OperationStatusFailure{Error: err}).Exec()
			}
			// new device here
			mountedBlockDevice = blockDevice
		}
	}

	// fail to mount
	if mountedBlockDevice == nil {
		err := errors.New("no new new device is found")
		return (&OperationStatusFailure{Error: err}).Exec()
	}

	// if disk is not format then format it
	if !vmdiskop.IsFormatted(mountedBlockDevice) {
		if operationMount.Options.FsType != "ext4" {
			err := errors.New(fmt.Sprintf("only support file format ext4, got: %s", operationMount.Options.FsType))
			return (&OperationStatusFailure{Error: err}).Exec()
		}

		// get disk uuid
		diskIdArr := strings.Split(diskForMount.Id, ":")
		uuid := diskIdArr[len(diskIdArr)-1]

		// format disk
		err := vmdiskop.FormatDeviceToExt4(mountedBlockDevice, operationMount.Options.PvOrVolumeName, uuid, time.Minute)
		if err != nil {
			return (&OperationStatusFailure{Error: err}).Exec()
		}
	}

	// mount disk
	err = vmdiskop.Mount(mountedBlockDevice, operationMount.MountDir, operationMount.Options.FsType, operationMount.Options.Readwrite)
	if err != nil {
		err := errors.New("no new new device is found")
		return (&OperationStatusFailure{Error: err}).Exec()
	}

	// output
	message, err := json.Marshal(struct {
		DiskId       string `json:"diskId"`
		DiskName     string `json:"diskName"`
		VmDeviceName string `json:"vmDeviceName"`
		MountPoint   string `json:"mountPoint"`
	}{
		DiskId:       diskForMount.Id,
		DiskName:     diskForMount.Name,
		VmDeviceName: mountedBlockDevice.Name,
		MountPoint:   operationMount.MountDir,
	})
	if err != nil {
		return (&OperationStatusFailure{Error: err}).Exec()
	}
	return &OperationExecResult{
		Status:  OperationExecResultStatusSuccess,
		Message: string(message),
	}, nil
}

type OperationUnmountA struct {
	Operation
	MountDir  string
	Options   *OperationOptions
	VcdConfig *OperationVcdConfig
}

func (operationUnmountA *OperationUnmountA) Exec() (*OperationExecResult, error) {
	// init vdc
	vdc, err := NewVdc(&VcdConfig{
		ApiEndpoint: operationUnmountA.VcdConfig.ApiEndpoint,
		Insecure:    operationUnmountA.VcdConfig.Insecure,
		User:        operationUnmountA.VcdConfig.User,
		Password:    operationUnmountA.VcdConfig.Password,
		Org:         operationUnmountA.VcdConfig.Org,
		Vdc:         operationUnmountA.VcdConfig.Vdc,
	})
	if err != nil {
		return (&OperationStatusFailure{Error: err}).Exec()
	}

	var diskForUnmount *VdcDisk
	// Find exists disk
	foundDisk, err := vdc.FindDiskByDiskName(operationUnmountA.Options.PvOrVolumeName)
	if err != nil {
		return (&OperationStatusFailure{Error: err}).Exec()
	} else {
		diskForUnmount = foundDisk
	}

	// find this VM is VDC
	vm, err := FindVm(vdc, operationUnmountA.VcdConfig.VdcVApp)
	if err != nil {
		return (&OperationStatusFailure{Error: err}).Exec()
	}

	// detach disk in vdc
	err = vdc.DetachDisk(vm, diskForUnmount)
	if err != nil {
		return (&OperationStatusFailure{Error: err}).Exec()
	}

	return &OperationExecResult{
		Status:  OperationExecResultStatusSuccess,
		Message: "OK",
	}, nil
}

type OperationUnmount struct {
	Operation
	MountDir  string
	VcdConfig *OperationVcdConfig
}

func (operationUnmount *OperationUnmount) Exec() (*OperationExecResult, error) {
	if operationUnmount.MountDir == "" {
		err := errors.New("mount dir is empty")
		return (&OperationStatusFailure{Error: err}).Exec()
	}

	// init vdc
	vdc, err := NewVdc(&VcdConfig{
		ApiEndpoint: operationUnmount.VcdConfig.ApiEndpoint,
		Insecure:    operationUnmount.VcdConfig.Insecure,
		User:        operationUnmount.VcdConfig.User,
		Password:    operationUnmount.VcdConfig.Password,
		Org:         operationUnmount.VcdConfig.Org,
		Vdc:         operationUnmount.VcdConfig.Vdc,
	})
	if err != nil {
		return (&OperationStatusFailure{Error: err}).Exec()
	}

	// find this VM is VDC
	vm, err := FindVm(vdc, operationUnmount.VcdConfig.VdcVApp)
	if err != nil {
		return (&OperationStatusFailure{Error: err}).Exec()
	}

	blockDevice, err := vmdiskop.FindDeviceByMountPoint(operationUnmount.MountDir)
	if err != nil {
		return (&OperationStatusFailure{Error: err}).Exec()
	}

	var diskForUnmount *VdcDisk
	// Find exists disk
	foundDisk, err := vdc.FindDiskByDiskName(blockDevice.Label)
	if err != nil {
		return (&OperationStatusFailure{Error: err}).Exec()
	} else {
		diskForUnmount = foundDisk
	}

	// unmount
	err = vmdiskop.Unmount(operationUnmount.MountDir)
	if err != nil {
		return (&OperationStatusFailure{Error: err}).Exec()
	}

	// remove scsi device
	err = vmdiskop.RemoveSCSIDevice(blockDevice)
	if err != nil {
		return (&OperationStatusFailure{Error: err}).Exec()
	}

	// detach disk in vdc
	err = vdc.DetachDisk(vm, diskForUnmount)
	if err != nil {
		return (&OperationStatusFailure{Error: err}).Exec()
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
		return (&OperationStatusFailure{Error: err}).Exec()
	}
	return &OperationExecResult{
		Status:  OperationExecResultStatusSuccess,
		Message: string(message),
	}, nil
}

type OperationStatusFailure struct {
	Operation
	Error       error
	OutputError error
}

func (operationStatusFailure *OperationStatusFailure) Exec() (*OperationExecResult, error) {
	var outputMessage string
	if operationStatusFailure.OutputError != nil {
		outputMessage = operationStatusFailure.OutputError.Error()
	} else if operationStatusFailure.Error != nil {
		b, err := json.Marshal(struct {
			Error string `json:"error"`
		}{
			Error: operationStatusFailure.Error.Error(),
		})

		if err != nil {
			outputMessage = fmt.Sprintf(`{"error": "%s"}`, err.Error())
		}

		outputMessage = string(b)
	}

	return &OperationExecResult{
		Status:  OperationExecResultStatusFailure,
		Message: outputMessage,
	}, operationStatusFailure.Error
}
