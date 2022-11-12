//go:build linux
// +build linux

package genetlink_test

import (
	"errors"
	"fmt"
	"net"
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

	// Pick families that are likely to exist on any given system. If they
	// don't, just skip the test. See:
	// https://github.com/mdlayher/genetlink/issues/7.
	families := []string{
		"nlctrl",
		"acpi_event",
	}

	for _, f := range families {
		if _, err := c.GetFamily(f); err != nil {
			t.Skipf("skipping, could not get family %q: %v", f, err)
		}
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

func TestIntegrationConnGetFamilyIsNotExist(t *testing.T) {
	// Test that the documented behavior of returning an error that is compatible
	// with netlink.IsNotExist is correct.
	const name = "NOTEXISTS"

	c, err := genetlink.Dial(nil)
	if err != nil {
		t.Fatalf("failed to dial generic netlink: %v", err)
	}

	if _, err := c.GetFamily(name); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected not exist error, got: %v", err)
	}

	if err := c.Close(); err != nil {
		t.Fatalf("error closing netlink connection: %v", err)
	}
}

func TestIntegrationConnGetFamily(t *testing.T) {
	c, err := genetlink.Dial(nil)
	if err != nil {
		t.Fatalf("failed to dial generic netlink: %v", err)
	}

	const name = "nlctrl"
	family, err := c.GetFamily(name)
	if err != nil {
		// nlctrl *should* always exist in order for genetlink to work at all.
		if errors.Is(err, os.ErrNotExist) {
			t.Fatal(errNLCtrlMissing)
		}

		t.Fatalf("failed to query for family: %v", err)
	}

	if err := c.Close(); err != nil {
		t.Fatalf("error closing netlink connection: %v", err)
	}

	if want, got := name, family.Name; want != got {
		t.Fatalf("unexpected family name:\n- want: %q\n-  got: %q", want, got)
	}
}

func TestIntegrationConnNL80211(t *testing.T) {
	c, err := genetlink.Dial(nil)
	if err != nil {
		t.Fatalf("failed to dial generic netlink: %v", err)
	}

	const (
		name = "nl80211"

		nl80211CommandGetInterface = 5

		nl80211AttributeInterfaceIndex = 3
		nl80211AttributeInterfaceName  = 4
		nl80211AttributeAttributeMAC   = 6
	)

	family, err := c.GetFamily(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			t.Skipf("skipping because %q family not available", name)
		}

		t.Fatalf("failed to query for family: %v", err)
	}

	req := genetlink.Message{
		Header: genetlink.Header{
			Command: nl80211CommandGetInterface,
			Version: family.Version,
		},
	}

	flags := netlink.Request | netlink.Dump
	msgs, err := c.Execute(req, family.ID, flags)
	if err != nil {
		t.Fatalf("failed to execute: %v", err)
	}

	if err := c.Close(); err != nil {
		t.Fatalf("error closing netlink connection: %v", err)
	}

	type ifInfo struct {
		Index int
		Name  string
		MAC   net.HardwareAddr
	}

	var infos []ifInfo
	for _, m := range msgs {
		ad, err := netlink.NewAttributeDecoder(m.Data)
		if err != nil {
			t.Fatalf("failed to create attribute decoder: %v", err)
		}

		var info ifInfo
		for ad.Next() {
			switch ad.Type() {
			case nl80211AttributeInterfaceIndex:
				info.Index = int(ad.Uint32())
			case nl80211AttributeInterfaceName:
				info.Name = ad.String()
			case nl80211AttributeAttributeMAC:
				ad.Do(func(b []byte) error {
					if l := len(b); l != 6 {
						return fmt.Errorf("unexpected MAC length: %d", l)
					}

					info.MAC = net.HardwareAddr(b)
					return nil
				})
			}
		}

		if err := ad.Err(); err != nil {
			t.Fatalf("failed to decode attributes: %v", err)
		}

		infos = append(infos, info)
	}

	// Verify that nl80211 reported the same information as package net
	for _, info := range infos {
		// TODO(mdlayher): figure out why nl80211 returns a subdevice with
		// an empty name on newer kernel
		if info.Name == "" {
			continue
		}

		ifi, err := net.InterfaceByName(info.Name)
		if err != nil {
			t.Fatalf("error retrieving interface %q: %v", info.Name, err)
		}

		if want, got := ifi.Index, info.Index; want != got {
			t.Fatalf("unexpected interface index for %q:\n- want: %v\n-  got: %v",
				ifi.Name, want, got)
		}

		if want, got := ifi.Name, info.Name; want != got {
			t.Fatalf("unexpected interface name:\n- want: %q\n-  got: %q",
				want, got)
		}

		if want, got := ifi.HardwareAddr.String(), info.MAC.String(); want != got {
			t.Fatalf("unexpected interface MAC for %q:\n- want: %q\n-  got: %q",
				ifi.Name, want, got)
		}
	}
}

func panicf(format string, a ...interface{}) {
	panic(fmt.Sprintf(format, a...))
}
