package utils

import (
	"fmt"
	"testing"

	"golang.org/x/sync/errgroup"
)

func TestCheckUDPPort(t *testing.T) {
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
