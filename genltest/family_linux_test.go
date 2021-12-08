//go:build linux
// +build linux

package genltest_test

import (
	"io"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/mdlayher/genetlink"
	"github.com/mdlayher/genetlink/genltest"
	"github.com/mdlayher/netlink"
	"github.com/mdlayher/netlink/nlenc"
	"github.com/mdlayher/netlink/nltest"
	"golang.org/x/sys/unix"
)

func TestServeFamily(t *testing.T) {
	tests := []struct {
		name string
		f    genetlink.Family
		fn   func(c *genetlink.Conn) (*genetlink.Family, error)
		ok   bool
		pass bool
	}{
		{
			name: "error, wrong attribute type",
			fn: func(c *genetlink.Conn) (*genetlink.Family, error) {
				m := genetlink.Message{
					Header: genetlink.Header{
						Command: unix.CTRL_CMD_GETFAMILY,
					},
					Data: nltest.MustMarshalAttributes([]netlink.Attribute{{
						Type: 0xff,
					}}),
				}

				_, err := c.Execute(m, unix.GENL_ID_CTRL, 0)
				return nil, err
			},
		},
		{
			name: "error, wrong family name",
			f:    genetlink.Family{Name: "foo"},
			fn: func(c *genetlink.Conn) (*genetlink.Family, error) {
				m := genetlink.Message{
					Header: genetlink.Header{
						Command: unix.CTRL_CMD_GETFAMILY,
					},
					Data: nltest.MustMarshalAttributes([]netlink.Attribute{{
						Type: unix.CTRL_ATTR_FAMILY_NAME,
						Data: nlenc.Bytes("bar"),
					}}),
				}

				_, err := c.Execute(m, unix.GENL_ID_CTRL, 0)
				return nil, err
			},
		},
		{
			name: "ok, family foo",
			f: genetlink.Family{
				ID:      1,
				Name:    "foo",
				Version: 1,
				Groups: []genetlink.MulticastGroup{
					{
						ID:   2,
						Name: "bar",
					},
					{
						ID:   3,
						Name: "baz",
					},
				},
			},
			fn: func(c *genetlink.Conn) (*genetlink.Family, error) {
				f, err := c.GetFamily("foo")
				return &f, err
			},
			ok: true,
		},
		{
			name: "pass, different family",
			fn: func(c *genetlink.Conn) (*genetlink.Family, error) {
				_, err := c.Execute(genetlink.Message{}, 2, 0)
				return nil, err
			},
			ok:   true,
			pass: true,
		},
		{
			name: "pass, different command",
			fn: func(c *genetlink.Conn) (*genetlink.Family, error) {
				m := genetlink.Message{
					Header: genetlink.Header{
						Command: unix.CTRL_CMD_DELFAMILY,
					},
				}

				_, err := c.Execute(m, unix.GENL_ID_CTRL, 0)
				return nil, err
			},
			ok:   true,
			pass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pass bool
			c := genltest.Dial(genltest.ServeFamily(tt.f,
				func(greq genetlink.Message, nreq netlink.Message) ([]genetlink.Message, error) {
					// Message was passed to inner handler.
					pass = true
					return nil, io.EOF
				},
			))
			defer c.Close()

			f, err := tt.fn(c)

			if err != nil && tt.ok {
				t.Fatalf("unexpected error: %v", err)
			}
			if err == nil && !tt.ok {
				t.Fatal("expected an error, but none occurred")
			}
			if err != nil {
				return
			}

			if !tt.pass {
				if diff := cmp.Diff(tt.f, *f); diff != "" {
					t.Fatalf("unexpected generic netlink family (-want +got):\n%s", diff)
				}
			}

			if diff := cmp.Diff(tt.pass, pass); diff != "" {
				t.Fatalf("unexpected function passthrough (-want +got):\n%s", diff)
			}
		})
	}
}
