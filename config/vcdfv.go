package config

type Vcdfv struct {
	VcdApiEndpoint string `yaml:"vcdApiEndpoint"`
	VcdInsecure    bool   `yaml:"vcdInsecure"`
	VcdUser        string `yaml:"vcdUser"`
	VcdPassword    string `yaml:"vcdPassword"`
	VcdOrg         string `yaml:"vcdOrg"`
	VcdVdc         string `yaml:"vcdVdc"`
	VcdVdcVApp     string `yaml:"vcdVdcVApp"`
}
