//go:build !linux
// +build !linux

package genltest

import (
	"fmt"
	"runtime"

	"github.com/mdlayher/genetlink"
	"github.com/mdlayher/netlink"
)

// errUnimplemented is returned by all functions on platforms that
// cannot make use of genltest.
var errUnimplemented = fmt.Errorf("genltest not implemented on %s/%s",
	runtime.GOOS, runtime.GOARCH)

// serveFamily returns a Func which always returns an error.
func serveFamily(f genetlink.Family, fn Func) Func {
	return func(_ genetlink.Message, _ netlink.Message) ([]genetlink.Message, error) {
		return nil, errUnimplemented
	}
}
