//go:build windows

package singleinstance

import (
	"errors"
	"fmt"
	"syscall"
)

const errorAlreadyExists syscall.Errno = 183

var (
	kernel32        = syscall.NewLazyDLL("kernel32.dll")
	procCreateMutex = kernel32.NewProc("CreateMutexW")
	procCloseHandle = kernel32.NewProc("CloseHandle")
)

func Acquire(name string) (func(), error) {
	wideName, err := syscall.UTF16PtrFromString(`Local\` + name)
	if err != nil {
		return nil, err
	}
	handle, _, callErr := procCreateMutex.Call(0, 0, uintptr(unsafePointer(wideName)))
	if handle == 0 {
		return nil, fmt.Errorf("CreateMutexW: %w", callErr)
	}
	if errors.Is(callErr, errorAlreadyExists) {
		procCloseHandle.Call(handle)
		return nil, errors.New("another AirDropPlus-Go instance is already running")
	}
	return func() { procCloseHandle.Call(handle) }, nil
}
