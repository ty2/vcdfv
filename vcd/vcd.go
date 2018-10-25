// Created by terry on 11/10/2018.

package vcd

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/vmware/go-vcloud-director/govcd"
	"github.com/vmware/go-vcloud-director/types/v56"
	"net/url"
	"time"
)

type VcdConfig struct {
	ApiEndpoint string
	Insecure    bool
	User        string
	Password    string
	Org         string
	Vdc         string
}

type VdcVApp struct {
	Id   string
	Name string
	Href string
	Vm   []*VAppVm
}

type VAppVm struct {
	Name string
	Href string
}

type VdcDisk struct {
	Id          string
	Name        string
	Href        string
	Size        int
	Description string
	Meta        *VdcDiskMeta
	AttachedVm  *DiskAttachedVm
}

type VdcDiskMeta struct {
	VmName     string    `json:"vmName"`
	DeviceName string    `json:"deviceName"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type DiskAttachedVm struct {
	Id   string
	Name string
}

type Vdc struct {
	vcdClient *govcd.VCDClient
	client    *govcd.Vdc
}

type DiskOpFn func(params *types.DiskAttachOrDetachParams) (govcd.Task, error)

func NewVdc(config *VcdConfig) (*Vdc, error) {
	// init VCD
	vdc := &Vdc{}

	// login to VCD
	client, err := vdc.connect(config)
	if err != nil {
		return nil, err
	}

	// assign VCD client
	vdc.vcdClient = client

	// get VDC info
	org, err := govcd.GetOrgByName(client, config.Org)
	if err != nil {
		return nil, err
	}

	// get VDC client
	vdcClient, err := org.GetVdcByName(config.Vdc)
	if err != nil {
		return nil, err
	}

	// assign VDC client
	vdc.client = &vdcClient

	return vdc, nil
}

func (vdc *Vdc) connect(config *VcdConfig) (*govcd.VCDClient, error) {
	// Parse API endpoint
	u, err := url.ParseRequestURI(config.ApiEndpoint)
	if err != nil {
		return nil, fmt.Errorf("unable to parse url: %s", err)
	}

	client := govcd.NewVCDClient(*u, config.Insecure)

	err = client.Authenticate(config.User, config.Password, config.Org)
	if err != nil {
		return nil, err
	}

	return client, nil
}

func (vdc *Vdc) FindVmByVAppNameAndVmName(vAppName string, vmName string) (*VAppVm, error) {
	vApp, err := vdc.client.FindVAppByName(vAppName)
	if err != nil {
		return nil, err
	}

	var vAppVm *VAppVm
	for _, vm := range vApp.VApp.Children.VM {
		if vm.Name == vmName {
			vAppVm = &VAppVm{
				Name: vm.Name,
				Href: vm.HREF,
			}
			break
		}
	}

	if vAppVm == nil {
		return nil, errors.New("not found")
	}

	return vAppVm, nil
}

func (vdc *Vdc) FindDiskByDiskName(diskName string) (*VdcDisk, error) {
	err := vdc.client.Refresh()
	if err != nil {
		return nil, err
	}

	var vdcDisk *VdcDisk
	for _, res := range vdc.client.Vdc.ResourceEntities {
		for _, item := range res.ResourceEntity {
			if item.Type == types.MimeDisk && item.Name == diskName {
				if vdcDisk != nil {
					return nil, errors.New(fmt.Sprintf("duplicate disk found, %s", diskName))
				}

				disk, err := vdc.client.FindDiskByHREF(item.HREF)
				if err != nil {
					return nil, err
				}

				vm, err := disk.AttachedVM()
				if err != nil {
					return nil, err
				}

				// if attached vm
				var diskAttachedVm *DiskAttachedVm
				if vm != nil {
					diskAttachedVm = &DiskAttachedVm{
						Id:   vm.ID,
						Name: vm.Name,
					}
				}

				vdcDisk = &VdcDisk{
					Id:          disk.Disk.Id,
					Name:        disk.Disk.Name,
					Size:        disk.Disk.Size,
					Description: disk.Disk.Description,
					Href:        disk.Disk.HREF,
					AttachedVm:  diskAttachedVm,
				}

				diskMeta, err := vdc.DiskMeta(vdcDisk)
				if err == nil {
					vdcDisk.Meta = diskMeta
				}
			}
		}
	}

	if vdcDisk == nil {
		return nil, errors.New("not found")
	}

	return vdcDisk, nil
}

func (vdc *Vdc) CreateDisk(disk *VdcDisk) (*VdcDisk, error) {
	vdcDisk, err := vdc.client.CreateDisk(&types.DiskCreateParams{
		Disk: &types.Disk{
			Name:        disk.Name,
			Size:        disk.Size,
			Description: disk.Description,
		},
	})

	disk.Href = vdcDisk.Disk.HREF

	task := govcd.NewTask(&vdc.vcdClient.Client)
	for _, taskItem := range vdcDisk.Disk.Tasks.Task {
		task.Task = taskItem
		err = task.WaitTaskCompletion()
	}

	if err != nil {
		return disk, err
	}

	return disk, nil
}

// Use independent disk's description as meta field
func (vdc *Vdc) DiskMeta(disk *VdcDisk) (*VdcDiskMeta, error) {
	var diskMeta *VdcDiskMeta

	err := json.Unmarshal([]byte(disk.Description), &diskMeta)
	if err != nil {
		return nil, err
	}

	return diskMeta, err
}

// Use independent disk's description as meta field
func (vdc *Vdc) SetDiskMeta(disk *VdcDisk, newDiskMeta *VdcDiskMeta) (*VdcDisk, error) {
	vcdDisk, err := vdc.client.FindDiskByHREF(disk.Href)
	if err != nil {
		return nil, err
	}

	// set date
	meta, err := vdc.DiskMeta(disk)
	if err == nil {
		// diskMeta is not set or unmarshal error
		newDiskMeta.CreatedAt = meta.UpdatedAt
		newDiskMeta.UpdatedAt = time.Now()
	} else {
		now := time.Now()
		newDiskMeta.CreatedAt = now
		newDiskMeta.UpdatedAt = now
	}

	b, err := json.Marshal(newDiskMeta)
	if err != nil {
		return nil, err
	}
	vcdDisk.Disk.Description = string(b)

	task, err := vcdDisk.Update(vcdDisk.Disk)
	if err != nil {
		return nil, err
	}

	err = task.WaitTaskCompletion()
	if err != nil {
		return nil, err
	}

	// return refreshed disk info
	return vdc.FindDiskByDiskName(vcdDisk.Disk.Name)
}

func (vdc *Vdc) DiskOp(disk *VdcDisk, busNumber int, unitNumber int, opFn DiskOpFn) error {
	if err := VerifyHref(disk.Href); err != nil {
		return err
	}

	diskAttachOrDetachParams := &types.DiskAttachOrDetachParams{
		Disk: &types.Reference{
			HREF: disk.Href,
		},
	}

	// set busNumber and unitNumber
	if busNumber >= 0 {
		diskAttachOrDetachParams.BusNumber = &busNumber
		if unitNumber >= 0 {
			diskAttachOrDetachParams.UnitNumber = &unitNumber
		}
	}

	task, err := opFn(diskAttachOrDetachParams)
	if err != nil {
		return err
	}
	err = task.WaitTaskCompletion()
	if err != nil {
		return err
	}

	return nil
}

func (vdc *Vdc) AttachDisk(vm *VAppVm, disk *VdcDisk, busNumber int, unitNumber int) error {
	if err := VerifyHref(vm.Href); err != nil {
		return err
	}

	vdcVM, err := vdc.vcdClient.FindVMByHREF(vm.Href)
	if err != nil {
		return err
	}

	err = vdc.DiskOp(disk, busNumber, unitNumber, vdcVM.AttachDisk)
	if err != nil {
		return err
	}

	return nil
}

func (vdc *Vdc) DetachDisk(vm *VAppVm, disk *VdcDisk) error {
	if err := VerifyHref(vm.Href); err != nil {
		return err
	}

	vdcVM, err := vdc.vcdClient.FindVMByHREF(vm.Href)
	if err != nil {
		return err
	}

	err = vdc.DiskOp(disk, -1, -1, vdcVM.DetachDisk)
	if err != nil {
		return err
	}

	return nil
}

func VerifyHref(href string) error {
	if href == "" {
		return errors.New("href is empty")
	}

	return nil
}
