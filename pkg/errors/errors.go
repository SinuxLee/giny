package errors

import (
	"fmt"
)

func New(code int, msg string) error {
	return &errorWithCode{
		code: code,
		msg:  msg,
	}
}

func Warp(code int, e error) error {
	return &errorWithCode{
		code: code,
		msg:  e.Error(),
	}
}

type errorWithCode struct {
	code int
	msg  string
}

func (e errorWithCode) Error() string {
	return fmt.Sprintf("code:%v,msg:%v", e.code, e.msg)
}

func Code(err error) int {
	if e, ok := err.(errorWithCode); ok {
		return e.code
	}
	return -1
}

func Message(err error) string {
	if e, ok := err.(errorWithCode); ok {
		return e.msg
	}
	return ""
}
