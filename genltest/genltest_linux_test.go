//+build linux

package genltest_test

import (
	"os"
	"syscall"
	"testing"

	"github.com/mdlayher/genetlink"
	"github.com/mdlayher/genetlink/genltest"
	"github.com/mdlayher/netlink"
)

func TestConnLinuxReceiveError(t *testing.T) {
	c := genltest.Dial(func(_ genetlink.Message, _ netlink.Message) ([]genetlink.Message, error) {
		return nil, genltest.Error(int(syscall.EPERM))
	})
	defer c.Close()

	// Send some generic request to enable the testing function to send
	// EPERM error back to us.
	if _, err := c.Send(genetlink.Message{}, 1, netlink.HeaderFlagsRequest); err != nil {
		t.Fatalf("failed to send: %v", err)
	}

	_, _, err := c.Receive()
	if !os.IsPermission(err) {
		t.Fatalf("expected permission denied error, but got: %v", err)
	}
}
