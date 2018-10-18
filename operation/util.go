package operation

import (
	"errors"
	"fmt"
	"github.com/ty2/vcdfv/config"
	"github.com/ty2/vcdfv/vcd"
	"os"
	"strconv"
)

func VdcClient(vcdfvConfig *config.Vcdfv) (*vcd.Vdc, error) {
	return vcd.NewVdc(&vcd.VcdConfig{
		ApiEndpoint: vcdfvConfig.VcdApiEndpoint,
		Insecure:    vcdfvConfig.VcdInsecure,
		User:        vcdfvConfig.VcdUser,
		Password:    vcdfvConfig.VcdPassword,
		Org:         vcdfvConfig.VcdOrg,
		Vdc:         vcdfvConfig.VcdVdc,
	})
}

func SizeStringToByteUnit(str string) (int, error) {
	// no unit in the string so it is byte then return byte size
	if size, err := strconv.ParseInt(str, 10, 0); err == nil {
		return int(size), nil
	}

	sizeString := str[:len(str)-1]
	unit := str[len(str)-1:]

	size, err := strconv.ParseFloat(sizeString, 64)
	if err != nil {
		return 0, errors.New(fmt.Sprintf("cannot parse size: %s", str))
	}

	switch unit {
	case "m": // MB
		size = size * 1024 * 1024
	case "g": // GB
		size = size * 1024 * 1024 * 1024
	}

	return int(size), nil
}

func FindVm(vdc *vcd.Vdc, vAppName string) (*vcd.VAppVm, error) {
	// get hostname
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}

	// find this server VM in VDC
	vm, err := vdc.FindVmByVAppNameAndVmName(vAppName, hostname)
	if err != nil {
		return nil, err
	}

	return vm, nil
}
