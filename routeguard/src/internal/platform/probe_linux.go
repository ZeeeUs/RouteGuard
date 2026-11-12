package platform

import "syscall"

func bindDevice(fd uintptr, device string) error {
	return syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, device)
}
