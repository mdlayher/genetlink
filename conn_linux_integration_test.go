//+build linux

package genetlink_test

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/mdlayher/genetlink"
	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

var errNLCtrlMissing = errors.New("family nlctrl appears to not exist, so " +
	"generic netlink is not functional on this machine; if you believe you " +
	"have received this message in error, please file an issue at " +
	"https://github.com/mdlayher/genetlink")

func TestIntegrationConnListFamilies(t *testing.T) {
	c, err := genetlink.Dial(nil)
	if err != nil {
		t.Fatalf("failed to dial generic netlink: %v", err)
	}

	families, err := c.ListFamilies()
	if err != nil {
		t.Fatalf("failed to query for families: %v", err)
	}

	if err := c.Close(); err != nil {
		t.Fatalf("error closing netlink connection: %v", err)
	}

	// Should be at least nlctrl present
	var found bool
	const name = "nlctrl"
	for _, f := range families {
		if f.Name == name {
			found = true
		}
	}

	if !found {
		t.Fatal(errNLCtrlMissing)
	}
}

func TestIntegrationConnConcurrentRaceFree(t *testing.T) {
	c, err := genetlink.Dial(nil)
	if err != nil {
		t.Fatalf("failed to dial generic netlink: %v", err)
	}

	execN := func(n int) {
		for i := 0; i < n; i++ {
			// Don't expect a "valid" request/reply because we are not serializing
			// our Send/Receive calls via Execute or with an external lock.
			//
			// Just verify that we don't trigger the race detector, we got a
			// valid netlink response, and it can be decoded as a valid
			// netlink message.
			if _, err := c.Send(genetlink.Message{}, 0, netlink.Request|netlink.Acknowledge); err != nil {
				panicf("failed to send request: %v", err)
			}

			msgs, _, err := c.Receive()
			if err != nil {
				panicf("failed to receive reply: %v", err)
			}

			if l := len(msgs); l != 1 {
				panicf("unexpected number of reply messages: %d", l)
			}
		}
	}

	const (
		workers    = 16
		iterations = 10000
	)

	var wg sync.WaitGroup
	wg.Add(workers)
	defer wg.Wait()

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			execN(iterations)
		}()
	}
}

func TestIntegrationConnConcurrentReceiveClose(t *testing.T) {
	c, err := genetlink.Dial(nil)
	if err != nil {
		t.Fatalf("failed to dial generic netlink: %v", err)
	}

	// Verify this test cannot block indefinitely due to Receive hanging after
	// a call to Close.
	timer := time.AfterFunc(10*time.Second, func() {
		panic("test took too long")
	})
	defer timer.Stop()

	var wg sync.WaitGroup
	wg.Add(1)
	defer wg.Wait()

	go func() {
		defer wg.Done()

		_, _, err := c.Receive()
		if err == nil {
			panicf("expected an error, but none occurred")
		}

		// Expect an error due to file descriptor being closed.
		serr := err.(*netlink.OpError).Err.(*os.SyscallError).Err
		if diff := cmp.Diff(unix.EBADF, serr); diff != "" {
			panicf("unexpected error from receive (-want +got):\n%s", diff)
		}
	}()

	if err := c.Close(); err != nil {
		t.Fatalf("failed to close: %v", err)
	}
}

func TestIntegrationConnConcurrentSerializeExecute(t *testing.T) {
	c, err := genetlink.Dial(nil)
	if err != nil {
		t.Fatalf("failed to dial generic netlink: %v", err)
	}

	execN := func(n int, family string) {
		for i := 0; i < n; i++ {
			// GetFamily will internally call Execute to ensure its
			// request/response transaction is serialized appropriately, and
			// any errors doing so will be reported here.
			f, err := c.GetFamily(family)
			if err != nil {
				panicf("failed to get family %q: %v", family, err)
			}

			if diff := cmp.Diff(family, f.Name); diff != "" {
				panicf("unexpected family name (-want +got):\n%s", diff)
			}
		}
	}

	const iterations = 2000

	// Pick families that are likely to exist on any given system.
	families := []string{
		"nlctrl",
		"acpi_event",
	}

	var wg sync.WaitGroup
	wg.Add(len(families))
	defer wg.Wait()

	for _, f := range families {
		go func(f string) {
			defer wg.Done()
			execN(iterations, f)
		}(f)
	}
}

func panicf(format string, a ...interface{}) {
	panic(fmt.Sprintf(format, a...))
}
