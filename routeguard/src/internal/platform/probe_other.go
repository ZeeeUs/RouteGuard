//go:build !linux

package platform

import "fmt"

func bindDevice(_ uintptr, _ string) error {
	return fmt.Errorf("WAN-bound diagnostics require Linux")
}
