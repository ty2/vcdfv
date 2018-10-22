// Created by terry on 11/10/2018.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/nightlyone/lockfile"
	"github.com/ty2/vcdfv/config"
	"github.com/ty2/vcdfv/operation"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FlexVolume Spec
// https://github.com/kubernetes/community/blob/f60e9ca9f800236e412104843e3a3ded908904c9/contributors/devel/flexvolume.md

var vcdfvConfig *config.Vcdfv

func init() {
	fileBytes, err := ioutil.ReadFile("/etc/kubernetes/vcdfv-config.yaml")
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
	lock, err := lockfile.New(filepath.Join(os.TempDir(), "lock.vcdfv.lck"))
	if err != nil {
		// cannot init lock
		result, err := (&operation.StatusFailure{
			Error: err,
		}).Exec()

		printResult(result, err)
		return
	}

	err = lockProcess(lock)
	if err != nil {
		// try to prevent long delay process timer in kubernetes, so it need to reduce fail times
		time.Sleep(time.Second * 30)

		result, err := (&operation.StatusFailure{
			Error: errors.New("waiting other process finish attach/detach disk"),
		}).Exec()
		printResult(result, err)
		return
	}

	defer lock.Unlock()

	args := os.Args

	// operation
	operation := argsToOperation(args)
	result, err := operation.Exec()
	printResult(result, err)
	return
}

func lockProcess(lock lockfile.Lockfile) error {
	err := lock.TryLock()
	// error handling is essential, as we only try to get the lock.
	if err != nil {
		return err
	}

	return nil
}

func printResult(result *operation.ExecResult, err error) {
	// echo result as possible
	if result != nil {
		// convert to join
		resultB, err := json.Marshal(result)
		if err != nil {
			log.Fatal(err)
		}
		// output
		fmt.Print(string(resultB))
		return
	} else if err != nil {
		// result is nil and err
		log.Output(2, err.Error())
		return
	}

}

func argsToOperation(args []string) operation.Operation {
	if len(args) <= 1 {
		err := errors.New("args len <= 1")
		return &operation.StatusFailure{
			Error: err,
		}
	}

	operationType := args[1]
	validOperations := map[string]func(args []string) operation.Operation{
		opInit:    argsToInitOperation,
		opMount:   argsToMountOperation,
		opUnmount: argsToUnMountOperation,
	}

	// verify operation type
	var op operation.Operation
	if opFn, ok := validOperations[operationType]; !ok {
		// invalid operation
		keys := make([]string, len(validOperations))
		i := 0
		for key := range validOperations {
			keys[i] = key
			i++
		}
		return &operation.StatusFailure{
			Error: NewError(errorInvalidOperationCode, fmt.Sprintf(errorInvalidOperationDefaultMsg, strings.Join(keys, ","), args[0])),
		}
	} else {
		// valid operation
		op = opFn(args)
	}

	// expect operation is not empty
	if op == nil {
		err := errors.New("operation is nil")
		return &operation.StatusFailure{
			Error: err,
		}
	}

	return op
}

func argsToInitOperation(args []string) operation.Operation {
	return &operation.Init{}
}

func argsToMountOperation(args []string) operation.Operation {
	if l := len(args); l != 4 {
		err := errors.New(fmt.Sprintf("args len != 4, got len: %v", l))
		return &operation.StatusFailure{
			Error: err,
		}
	}

	option := &operation.Options{}
	err := json.Unmarshal([]byte(args[3]), option)
	if err != nil {
		return &operation.StatusFailure{
			Error: err,
		}
	}

	return &operation.Mount{
		MountDir:    args[2],
		Options:     option,
		VcdfvConfig: vcdfvConfig,
	}
}

func argsToUnMountOperation(args []string) operation.Operation {
	if l := len(args); l < 3 {
		err := errors.New(fmt.Sprintf("args len < 3, got len: %v", l))
		return &operation.StatusFailure{
			Error: err,
		}
	}

	return &operation.Unmount{
		MountDir:    args[2],
		VcdfvConfig: vcdfvConfig,
	}
}
