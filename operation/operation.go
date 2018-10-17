// Created by terry on 11/10/2018.

package operation

type Operation interface {
	Exec() (*ExecResult, error)
}

const (
	ExecResultStatusSuccess      = "Success"
	ExecResultStatusFailure      = "Failure"
	ExecResultStatusNotSupported = "Not supported"
)

type VcdConfig struct {
	ApiEndpoint string `yaml:"apiEndpoint"`
	Insecure    bool   `yaml:"insecure"`
	User        string `yaml:"user"`
	Password    string `yaml:"password"`
	Org         string `yaml:"org"`
	Vdc         string `yaml:"vdc"`
	VdcVApp     string `yaml:"vdcVApp"`
}

type ExecResult struct {
	Status       string            `json:"status"`
	Message      string            `json:"message"`
	Capabilities *ExecCapabilities `json:"capabilities,omitempty"`
}

type ExecCapabilities struct {
	Attach bool `json:"attach"`
}

type Options struct {
	FsType         string `json:"kubernetes.io/fsType"`
	Readwrite      string `json:"kubernetes.io/readwrite"`
	FsGroup        string `json:"kubernetes.io/fsGroup"`
	PvOrVolumeName string `json:"kubernetes.io/pvOrVolumeName"`
	// Addition Options
	DiskInitialSize string `json:"diskInitialSize"`
}
