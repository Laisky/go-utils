package utils

import (
	"fmt"
	"testing"

	"golang.org/x/sync/errgroup"
)

func TestCheckUDPPort(t *testing.T) {
	var pool errgroup.Group
	for port := 1; port < 100; port++ {
		port := port
		pool.Go(func() error {
			if err := IsRemoteUDPPortOpen(fmt.Sprintf("scanme.nmap.org:%d", port)); err != nil {
				return err
			}

			fmt.Println(port)
			return nil
		})
	}

	_ = pool.Wait()
	// t.Error()
}
