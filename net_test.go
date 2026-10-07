package utils

import (
	"fmt"
	"testing"

	"golang.org/x/sync/errgroup"
)

// TestCheckUDPPort exercises IsRemoteUDPPortOpen concurrently against UDP ports 1 through 9 of 1.2.3.4 and prints
// each port reported as open. It is a smoke test only: probe errors are ignored and nothing is asserted.
func TestCheckUDPPort(t *testing.T) {
	t.Parallel()

	var pool errgroup.Group
	for port := 1; port < 10; port++ {
		port := port
		pool.Go(func() error {
			if err := IsRemoteUDPPortOpen(fmt.Sprintf("1.2.3.4:%d", port)); err != nil {
				return err
			}

			fmt.Println(port)
			return nil
		})
	}

	_ = pool.Wait()
	// t.Error()
}
