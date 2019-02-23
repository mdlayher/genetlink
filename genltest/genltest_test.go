package genltest_test

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/mdlayher/genetlink"
	"github.com/mdlayher/genetlink/genltest"
	"github.com/mdlayher/netlink"
)

func TestConnSend(t *testing.T) {
	req := genetlink.Message{
		Data: []byte{0xff, 0xff, 0xff, 0xff},
	}

	c := genltest.Dial(func(creq genetlink.Message, _ netlink.Message) ([]genetlink.Message, error) {
		if want, got := req.Data, creq.Data; !bytes.Equal(want, got) {
			t.Fatalf("unexpected request data:\n- want: %v\n-  got: %v",
				want, got)
		}

		return nil, nil
	})
	defer c.Close()

	if _, err := c.Send(req, 1, 1); err != nil {
		t.Fatalf("failed to send request: %v", err)
	}
}

func TestConnExecuteOK(t *testing.T) {
	req := genetlink.Message{
		Data: []byte{0xff},
	}

	c := genltest.Dial(func(creq genetlink.Message, _ netlink.Message) ([]genetlink.Message, error) {
		// Turn the request back around to the client.
		return []genetlink.Message{creq}, nil
	})
	defer c.Close()

	got, err := c.Execute(req, 1, 1)
	if err != nil {
		t.Fatalf("failed to execute request: %v", err)
	}

	if want := []genetlink.Message{req}; !reflect.DeepEqual(want, got) {
		t.Fatalf("unexpected response messages:\n- want: %v\n-  got: %v",
			want, got)
	}
}

func TestConnExecuteNoMessages(t *testing.T) {
	c := genltest.Dial(func(_ genetlink.Message, _ netlink.Message) ([]genetlink.Message, error) {
		return nil, io.EOF
	})
	defer c.Close()

	msgs, err := c.Execute(genetlink.Message{}, 0, 0)
	if err != nil {
		t.Fatalf("failed to execute: %v", err)
	}

	if l := len(msgs); l > 0 {
		t.Fatalf("expected no generic netlink messages, but got: %d", l)
	}
}

func TestConnReceiveNoMessages(t *testing.T) {
	c := genltest.Dial(func(_ genetlink.Message, _ netlink.Message) ([]genetlink.Message, error) {
		return nil, io.EOF
	})
	defer c.Close()

	gmsgs, nmsgs, err := c.Receive()
	if err != nil {
		t.Fatalf("failed to execute: %v", err)
	}

	if l := len(gmsgs); l > 0 {
		t.Fatalf("expected no generic netlink messages, but got: %d", l)
	}

	if l := len(nmsgs); l > 0 {
		t.Fatalf("expected no netlink messages, but got: %d", l)
	}
}

func TestConnReceiveError(t *testing.T) {
	errFoo := errors.New("foo")

	c := genltest.Dial(func(_ genetlink.Message, _ netlink.Message) ([]genetlink.Message, error) {
		return nil, errFoo
	})
	defer c.Close()

	_, _, err := c.Receive()
	if err != errFoo {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckRequest(t *testing.T) {
	tests := []struct {
		name    string
		family  uint16
		command uint8
		flags   netlink.HeaderFlags
		greq    genetlink.Message
		nreq    netlink.Message
		ok      bool
	}{
		{
			name: "no checking",
			ok:   true,
		},
		{
			name:   "bad family",
			family: 1,
			nreq: netlink.Message{
				Header: netlink.Header{
					// genetlink family is netlink header type.
					Type: 2,
				},
			},
		},
		{
			name:  "bad flags",
			flags: netlink.Request,
			nreq: netlink.Message{
				Header: netlink.Header{
					Flags: netlink.Replace,
				},
			},
		},
		{
			name:    "bad command",
			command: 1,
			greq: genetlink.Message{
				Header: genetlink.Header{
					Command: 2,
				},
			},
		},
		{
			name:    "ok",
			family:  1,
			command: 1,
			flags:   netlink.Request,
			nreq: netlink.Message{
				Header: netlink.Header{
					Type:  1,
					Flags: netlink.Request,
				},
			},
			greq: genetlink.Message{
				Header: genetlink.Header{
					Command: 1,
				},
			},
			ok: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn := genltest.CheckRequest(tt.family, tt.command, tt.flags, noop)
			_, err := fn(tt.greq, tt.nreq)

			if err != nil && tt.ok {
				t.Fatalf("unexpected error: %v", err)
			}
			if err == nil && !tt.ok {
				t.Fatal("expected an error, but none occurred")
			}
		})
	}
}

var noop = func(greq genetlink.Message, nreq netlink.Message) ([]genetlink.Message, error) {
	return nil, nil
}
