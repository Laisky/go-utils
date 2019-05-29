package utils_test

import (
	"sync"
	"testing"

	utils "github.com/Laisky/go-utils"
)

func ExampleCounter() {
	counter := utils.NewCounter()
	counter.Count()
	counter.CountN(10)
	counter.Get() // get current count
}

func ExampleRotateCounter() {
	counter, err := utils.NewRotateCounter(10)
	if err != nil {
		panic(err)
	}

	counter.Count()    // 1
	counter.CountN(10) // 1

}

func TestCounter(t *testing.T) {
	counter := utils.NewCounterFromN(0)
	counter = utils.NewCounter()
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
	counter, err := utils.NewRotateCounterFromN(100, 10)
	if err == nil {
		t.Fatal("should got error")
	}

	counter, err = utils.NewRotateCounter(10)
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
	if r = counter.CountN(248); r != 3 {
		t.Errorf("want %v, got %v", 3, r)
	}
}

func TestIncrementRotateCounter(t *testing.T) {
	utils.SetupLogger("debug")
	counter, err := utils.NewMonotonicRotateCounter(100)
	if err != nil {
		t.Fatalf("got error: %+v", err)
	}

	var (
		start, got, step int64
	)
	if got = counter.Count(); got-start < 1 {
		t.Errorf("%v should bigger than %v", got, start)
	}
	start = got

	if got = counter.Count(); got-start < 1 {
		t.Errorf("%v should bigger than %v", got, start)
	}
	start = got

	if got = counter.Count(); got-start < 1 {
		t.Errorf("%v should bigger than %v", got, start)
	}
	start = got

	step = 4
	if got = counter.CountN(step); got-start < step {
		t.Errorf("%v should bigger than %v", got, start)
	}
	start = got

	step = 15
	if got = counter.CountN(step); got-start < step {
		t.Errorf("%v should bigger than %v", got, start)
	}
	start = got

	step = 110
	if got = counter.CountN(step); got > step+start%100 {
		t.Errorf("%v should bigger than %v", got, step+start%100)
	}
	start = got

	// test duplicate
	if counter, err = utils.NewMonotonicRotateCounter(10000000); err != nil {
		t.Fatalf("got error: %+v", err)
	}

	ns := sync.Map{}
	wg := sync.WaitGroup{}
	val := struct{}{}
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 1000000; i++ {
			n := counter.Count()
			if _, ok := ns.LoadOrStore(n, val); ok {
				t.Fatalf("should not contains: %v", n)
			}
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			n := counter.CountN(500)

			if _, ok := ns.LoadOrStore(n, val); ok {
				t.Fatalf("should not contains: %v", n)
			}
		}
	}()

	wg.Wait()

}

func TestRotateCounterFromN(t *testing.T) {
	counter, err := utils.NewRotateCounterFromN(2, 10)
	if err != nil {
		t.Fatalf("got error: %+v", err)
	}

	var r int64
	if r = counter.Count(); r != 2 {
		t.Errorf("want %v, got %v", 2, r)
	}
	if r = counter.Count(); r != 3 {
		t.Errorf("want %v, got %v", 3, r)
	}
	if r = counter.Count(); r != 4 {
		t.Errorf("want %v, got %v", 4, r)
	}
	if r = counter.CountN(3); r != 7 {
		t.Errorf("want %v, got %v", 7, r)
	}
	if r = counter.CountN(10); r != 7 {
		t.Errorf("want %v, got %v", 7, r)
	}
}

func BenchmarkCounter(b *testing.B) {
	counter := utils.NewCounter()

	b.Run("count 1", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				counter.Count()
			}
		})
	})
	b.Run("get speed", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				counter.GetSpeed()
			}
		})
	})
	b.Run("count 1 parallel 4", func(b *testing.B) {
		for i := 0; i < 4; i++ {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					counter.Count()
				}
			})
		}
	})
	b.Run("count 5", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			counter.CountN(5)
		}
	})
	b.Run("count 500", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			counter.CountN(500)
		}
	})
	b.Run("count 500 parallel 4", func(b *testing.B) {
		for i := 0; i < 4; i++ {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					counter.CountN(500)
				}
			})
		}
	})
}

func BenchmarkRotateCounter(b *testing.B) {
	counter, err := utils.NewRotateCounter(1000000000)
	if err != nil {
		b.Fatalf("got error: %+v", err)
	}
	b.Run("count 1", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				counter.Count()
			}
		})
	})
	b.Run("count 1 parallel 4", func(b *testing.B) {
		for i := 0; i < 4; i++ {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					counter.Count()
				}
			})
		}
	})
	b.Run("count 5", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			counter.CountN(5)
		}
	})
	b.Run("count 500", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			counter.CountN(500)
		}
	})
	b.Run("count 500 parallel 4", func(b *testing.B) {
		for i := 0; i < 4; i++ {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					counter.CountN(500)
				}
			})
		}
	})
}

func BenchmarkIncrementalRotateCounter(b *testing.B) {
	counter, err := utils.NewMonotonicRotateCounter(1000000000)
	if err != nil {
		b.Fatalf("got error: %+v", err)
	}
	b.Run("count 1", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				counter.Count()
			}
		})
	})
	b.Run("count 1 parallel 4", func(b *testing.B) {
		for i := 0; i < 4; i++ {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					counter.Count()
				}
			})
		}
	})
	b.Run("count 5", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			counter.CountN(5)
		}
	})
	b.Run("count 500", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			counter.CountN(500)
		}
	})
	b.Run("count 500 parallel 4", func(b *testing.B) {
		for i := 0; i < 4; i++ {
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					counter.CountN(500)
				}
			})
		}
	})
}
