// Created by terry on 11/10/2018.

package vcd

import (
	"errors"
	"fmt"
	"github.com/vmware/go-vcloud-director/govcd"
	"github.com/vmware/go-vcloud-director/types/v56"
	"net/url"
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
	AttachedVm  *DiskAttachedVm
}

type DiskAttachedVm struct {
	Id   string
	Name string
}

type Vdc struct {
	vcdClient *govcd.VCDClient
	client    *govcd.Vdc
}

type DiskOpFn func(*govcd.Disk) (govcd.Task, error)

func NewVdc(config *VcdConfig) (*Vdc, error) {

	// Init VCD
	vdc := &Vdc{}

	// Login to VCD
	client, err := vdc.connect(config)
	if err != nil {
		return nil, err
	}

	// Assign VCD client
	vdc.vcdClient = client

	// Get VDC info
	org, err := govcd.GetOrgByName(client, config.Org)
	if err != nil {
		return nil, err
	}

	// Get VDC client
	vdcClient, err := org.GetVdcByName(config.Vdc)
	if err != nil {
		return nil, err
	}

	// Assign VDC client
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
					Id:         disk.Disk.Id,
					Name:       disk.Disk.Name,
					Size:       disk.Disk.Size,
					Href:       disk.Disk.HREF,
					AttachedVm: diskAttachedVm,
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
	vdcDisk, err := vdc.client.CreateDisk(&types.DiskCreateParamsDisk{
		Name:        disk.Name,
		Size:        disk.Size,
		Description: disk.Description,
	})

	disk.Href = vdcDisk.Disk.HREF

	task := govcd.NewTask(&vdc.vcdClient.Client)
	for _, taskItem := range vdcDisk.Disk.Tasks {
		task.Task = taskItem
		err = task.WaitTaskCompletion()
	}

	if err != nil {
		return disk, err
	}

	return disk, nil
}

func (vdc *Vdc) DeleteDisk(disk *VdcDisk) error {
	if err := VerifyHref(disk.Href); err != nil {
		return err
	}

	vcdDisk, err := vdc.client.FindDiskByHREF(disk.Href)
	if err != nil {
		return err
	}

	task, err := vcdDisk.Delete()
	if err != nil {
		return err
	}

	err = task.WaitTaskCompletion()
	if err != nil {
		return err
	}

	return nil
}

func (vdc *Vdc) DiskOp(disk *VdcDisk, opFn DiskOpFn) error {
	if err := VerifyHref(disk.Href); err != nil {
		return err
	}

	vdcDisk, err := vdc.client.FindDiskByHREF(disk.Href)
	if err != nil {
		return err
	}

	task, err := opFn(vdcDisk)
	if err != nil {
		return err
	}

	err = task.WaitTaskCompletion()
	if err != nil {
		return err
	}

	return nil
}

func (vdc *Vdc) AttachDisk(vm *VAppVm, disk *VdcDisk) error {
	if err := VerifyHref(vm.Href); err != nil {
		return err
	}

	vdcVM, err := vdc.vcdClient.FindVMByHREF(vm.Href)
	if err != nil {
		return err
	}

	err = vdc.DiskOp(disk, vdcVM.AttachDisk)
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

	err = vdc.DiskOp(disk, vdcVM.DetachDisk)
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
