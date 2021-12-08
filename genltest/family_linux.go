//go:build linux
// +build linux

package genltest

import (
	"fmt"

	"github.com/mdlayher/genetlink"
	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

// serveFamily is the Linux implementation of ServeFamily.
func serveFamily(f genetlink.Family, fn Func) Func {
	return func(greq genetlink.Message, nreq netlink.Message) ([]genetlink.Message, error) {
		// Only intercept "get family" commands to the generic netlink controller.
		if nreq.Header.Type != unix.GENL_ID_CTRL || greq.Header.Command != unix.CTRL_CMD_GETFAMILY {
			return fn(greq, nreq)
		}

		ad, err := netlink.NewAttributeDecoder(greq.Data)
		if err != nil {
			return nil, fmt.Errorf("genltest: failed to parse get family request attributes: %v", err)
		}

		// Ensure this request is for the family provided by f.
		for ad.Next() {
			if want, got := unix.CTRL_ATTR_FAMILY_NAME, int(ad.Type()); want != got {
				return nil, fmt.Errorf("genltest: unexpected get family request attribute: %d, want: %d", got, want)
			}

			if want, got := f.Name, ad.String(); want != got {
				return nil, fmt.Errorf("genltest: unexpected get family request value: %q, want: %q", got, want)
			}
		}

		if err := ad.Err(); err != nil {
			return nil, fmt.Errorf("genltest: unexpected error decoding get family request: %v", err)
		}

		// Return the family information for f.
		ae := netlink.NewAttributeEncoder()
		ae.Uint16(unix.CTRL_ATTR_FAMILY_ID, f.ID)
		ae.String(unix.CTRL_ATTR_FAMILY_NAME, f.Name)
		ae.Uint32(unix.CTRL_ATTR_VERSION, uint32(f.Version))

		// Encode multicast group attributes if applicable.
		if len(f.Groups) > 0 {
			ae.Nested(unix.CTRL_ATTR_MCAST_GROUPS, encodeGroups(f.Groups))
		}

		attrb, err := ae.Encode()
		if err != nil {
			return nil, err
		}

		return []genetlink.Message{{
			Header: genetlink.Header{
				Command: unix.CTRL_CMD_NEWFAMILY,
				// TODO(mdlayher): constant nlctrl version number?
				Version: 2,
			},
			Data: attrb,
		}}, nil
	}
}

// encodeGroups encodes multicast groups as packed netlink attributes.
func encodeGroups(groups []genetlink.MulticastGroup) func(ae *netlink.AttributeEncoder) error {
	return func(ae *netlink.AttributeEncoder) error {
		// Groups are a netlink "array" of nested attributes.
		for i, g := range groups {
			ae.Nested(uint16(i), func(nae *netlink.AttributeEncoder) error {
				nae.String(unix.CTRL_ATTR_MCAST_GRP_NAME, g.Name)
				nae.Uint32(unix.CTRL_ATTR_MCAST_GRP_ID, g.ID)
				return nil
			})
		}

		return nil
	}
}
