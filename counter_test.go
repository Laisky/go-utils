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

func TestUint32Counter(t *testing.T) {
	counter := utils.NewUint32Counter()
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

func TestRotateCounter(t *testing.T) {
	counter, err := utils.NewRotateCounter(10)
	if err != nil {
		t.Fatalf("got error: %+v", err)
	}

	var r int64
	if r = counter.Count(); r != 0 {
		t.Errorf("want %v, got %v", 0, r)
	}
	if r = counter.Count(); r != 1 {
		t.Errorf("want %v, got %v", 1, r)
	}
	if r = counter.Count(); r != 2 {
		t.Errorf("want %v, got %v", 2, r)
	}
	if r = counter.CountN(3); r != 5 {
		t.Errorf("want %v, got %v", 5, r)
	}
	if r = counter.CountN(10); r != 5 {
		t.Errorf("want %v, got %v", 5, r)
	}
}

func BenchmarkRotateCounter(b *testing.B) {
	counter, err := utils.NewRotateCounter(1000000)
	if err != nil {
		b.Fatalf("got error: %+v", err)
	}
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			counter.Count()
		}
	})
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			counter.CountN(5)
		}
	})
}
