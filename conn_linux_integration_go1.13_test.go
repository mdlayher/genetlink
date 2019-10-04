//+build linux,go1.13

package genetlink_test

import (
	"errors"
	"fmt"
	"net"
	"os"
	"testing"

	"github.com/mdlayher/genetlink"
	"github.com/mdlayher/netlink"
)

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
