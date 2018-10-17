// Created by terry on 11/10/2018.
// TODO apply custom error to operations, vcd

package main

import "fmt"

type Error struct {
	code    string
	message string
}

func NewError(code string, message string) *Error {
	return &Error{code: code, message: message}
}

func (e *Error) Error() string {
	return fmt.Sprintf("[%s] %s", e.code, e.message)
}

func (e *Error) Code() string {
	return e.code
}

func (e *Error) Message() string {
	return e.code
}
