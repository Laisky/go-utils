package utils

import (
	"net"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// IsRemoteUDPPortOpen check is remote udp port open
//
// Args:
//   addr: ""
func IsRemoteUDPPortOpen(addr string) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return errors.WithStack(err)
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return errors.WithStack(err)
	}
	defer conn.Close()

	if err = conn.SetDeadline(Clock.GetUTCNow().Add(3 * time.Second)); err != nil {
		return errors.WithStack(err)
	}

	if _, err = conn.Write([]byte("0")); err != nil {
		return errors.Wrap(err, "write")
	}

	data := make([]byte, 1)
	if _, _, err = conn.ReadFromUDP(data); err != nil {
		if strings.Contains(err.Error(), "i/o timeout") {
			return nil
		}

		return errors.WithStack(err)
	}

	return nil
}
