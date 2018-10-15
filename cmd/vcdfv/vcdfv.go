// Created by terry on 11/10/2018.

package main

import (
	"encoding/json"
	"fmt"
	"github.com/go-siris/siris/core/errors"
	"github.com/ty2/vcdfv"
	"log"
	"os"
	"strings"
)

// FlexVolume Spec
// https://github.com/kubernetes/community/blob/f60e9ca9f800236e412104843e3a3ded908904c9/contributors/devel/flexvolume.md

func main() {
	args := os.Args

	// operation
	operation := argsToOperation(args)
	result, err := operation.Exec()

	// echo result as possible
	if result != nil {
		// convert to join
		resultB, err := json.Marshal(result)
		if err != nil {
			log.Fatal(err)
		}
		// output
		fmt.Print(string(resultB))
		os.Exit(0)
	} else if err != nil {
		// result is nil and err
		log.Fatal(err)
	}

	fmt.Println("should not run this one")
}

func argsToOperation(args []string) vcdfv.Operation {
	if len(args) <= 1 {
		err := errors.New("args len <= 1")
		return &vcdfv.OperationStatusFailure{
			Error: err,
		}
	}

	operation := args[1]
	validOperations := map[string]func(args []string) vcdfv.Operation{
		opInit:     argsToInitOperation,
		opMount:    argsToMountOperation,
		opUnmount:  argsToUnMountOperation,
		opUnmountA: argsToUnMountAOperation,
	}

	// check valid operation
	var op vcdfv.Operation
	if opFn, ok := validOperations[operation]; !ok {
		// invalid operation
		keys := make([]string, len(validOperations))
		i := 0
		for key := range validOperations {
			keys[i] = key
			i++
		}
		return &vcdfv.OperationStatusFailure{
			Error: vcdfv.NewError(errorInvalidOperationCode, fmt.Sprintf(errorInvalidOperationDefaultMsg, strings.Join(keys, ","), args[0])),
		}
	} else {
		// valid operation
		op = opFn(args)
	}

	// expect operation is not empty
	if op == nil {
		err := errors.New("operation is nil")
		return &vcdfv.OperationStatusFailure{
			Error: err,
		}
	}

	return op
}

func argsToInitOperation(args []string) vcdfv.Operation {
	return &vcdfv.OperationInit{}
}

func argsToMountOperation(args []string) vcdfv.Operation {
	if l := len(args); l != 4 {
		err := errors.New(fmt.Sprintf("args len != 4, got len: %v", l))
		return &vcdfv.OperationStatusFailure{
			Error: err,
		}
	}

	option := &vcdfv.OperationOptions{}
	err := json.Unmarshal([]byte(args[3]), option)
	if err != nil {
		return &vcdfv.OperationStatusFailure{
			Error: err,
		}
	}

	return &vcdfv.OperationMount{
		MountDir: args[2],
		Options:  option,
	}
}

func argsToUnMountOperation(args []string) vcdfv.Operation {
	if l := len(args); l < 3 {
		err := errors.New(fmt.Sprintf("args len < 3, got len: %v", l))
		return &vcdfv.OperationStatusFailure{
			Error: err,
		}
	}

	option := &vcdfv.OperationOptions{}
	err := json.Unmarshal([]byte(args[3]), option)
	if err != nil {
		return nil
	}

	return &vcdfv.OperationUnmount{
		MountDir: args[2],
		Options:  option,
	}
}

func argsToUnMountAOperation(args []string) vcdfv.Operation {
	if l := len(args); l < 3 {
		err := errors.New(fmt.Sprintf("args len < 3, got len: %v", l))
		return &vcdfv.OperationStatusFailure{
			Error: err,
		}
	}

	option := &vcdfv.OperationOptions{}
	err := json.Unmarshal([]byte(args[3]), option)
	if err != nil {
		return nil
	}

	return &vcdfv.OperationUnmountA{
		MountDir: args[2],
		Options:  option,
	}
}
