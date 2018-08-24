package utils_test

import (
	"sync"
	"testing"

	utils "github.com/Laisky/go-utils"
)

func TestCounter(t *testing.T) {
	counter := utils.NewCounter()
	wg := &sync.WaitGroup{}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				counter.Count()
			}
		}()
	}

	wg.Wait()
	if counter.Get() != 10000 {
		t.Errorf("expect 10000, got %v", counter.Get())
	}

	counter.Set(10)
	if counter.Get() != 10 {
		t.Errorf("expect 10, got %v", counter.Get())
	}
}
