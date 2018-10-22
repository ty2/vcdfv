package main

import (
	"github.com/ty2/vcdfv/config"
	"github.com/ty2/vcdfv/operation"
	"github.com/ty2/vcdfv/vcd"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
)

var vcdfvConfig *config.Vcdfv

func init() {
	fileBytes, err := ioutil.ReadFile("vcdfv-config.yaml")
	if err != nil {
		output, _ := (&operation.StatusFailure{
			Error: err,
		}).Exec()
		log.Fatal(output)
	}

	err = yaml.Unmarshal([]byte(fileBytes), &vcdfvConfig)
	if err != nil {
		output, _ := (&operation.StatusFailure{
			Error: err,
		}).Exec()
		log.Fatal(output)
	}
}

func main() {
	vdc, err := vcd.NewVdc(&vcd.VcdConfig{
		ApiEndpoint: vcdfvConfig.VcdApiEndpoint,
		Insecure:    vcdfvConfig.VcdInsecure,
		User:        vcdfvConfig.VcdUser,
		Password:    vcdfvConfig.VcdPassword,
		Org:         vcdfvConfig.VcdOrg,
		Vdc:         vcdfvConfig.VcdVdc,
	})

	if err != nil {
		panic(err)
	}

	vm, err := vdc.FindVmByVAppNameAndVmName("kube-1", "kube-1-worker-1")
	if err != nil {
		panic(err)
	}

	disk, err := vdc.FindDiskByDiskName("kci-wordpress-mysql")
	if err != nil {
		panic(err)
	}

	vdc.DetachDisk(vm, disk)
}
