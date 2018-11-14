// Package journal_test
// `go test -bench=. -benchtime=30s | grep -E "/op|Benchmark"`
package journal_test

import (
	"bufio"
	"io/ioutil"
	"os"
	"testing"

	utils "github.com/Laisky/go-utils"
)

func BenchmarkWrite(b *testing.B) {
	fp, err := ioutil.TempFile("", "fs-test")
	if err != nil {
		b.Fatalf("%+v", err)
	}
	// fp, err := os.OpenFile("/data/go/src/github.com/Laisky/go-utils/journal/benchmark/test/test.data", os.O_RDWR|os.O_CREATE, 0664)
	// if err != nil {
	// 	b.Fatalf("got error: %+v", err)
	// }
	defer fp.Close()
	defer os.Remove(fp.Name())
	b.Logf("create file name: %v", fp.Name())

	data2K := []byte(utils.RandomStringWithLength(2048))
	b.Run("direct write", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err = fp.Write(data2K); err != nil {
				b.Fatalf("got error: %+v", err)
			}
		}
	})

	fpBuf := bufio.NewWriter(fp)
	b.Run("write default buf", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err = fpBuf.Write(data2K); err != nil {
				b.Fatalf("got error: %+v", err)
			}
		}
	})

	b.Run("write default buf with flush", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err = fpBuf.Write(data2K); err != nil {
				b.Fatalf("got error: %+v", err)
			}
			if err = fpBuf.Flush(); err != nil {
				b.Fatalf("got error: %+v", err)
			}
		}
	})

	fpBuf4KB := bufio.NewWriterSize(fp, 1024*4)
	b.Run("write 4KB buf", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err = fpBuf4KB.Write(data2K); err != nil {
				b.Fatalf("got error: %+v", err)
			}
			if err = fpBuf4KB.Flush(); err != nil {
				b.Fatalf("got error: %+v", err)
			}
		}
	})

	fpBuf8KB := bufio.NewWriterSize(fp, 1024*8)
	b.Run("write 8KB buf", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err = fpBuf8KB.Write(data2K); err != nil {
				b.Fatalf("got error: %+v", err)
			}
			if err = fpBuf8KB.Flush(); err != nil {
				b.Fatalf("got error: %+v", err)
			}
		}
	})

	fpBuf16KB := bufio.NewWriterSize(fp, 1024*16)
	b.Run("write 16KB buf", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err = fpBuf16KB.Write(data2K); err != nil {
				b.Fatalf("got error: %+v", err)
			}
			if err = fpBuf16KB.Flush(); err != nil {
				b.Fatalf("got error: %+v", err)
			}
		}
	})

	fpBuf1M := bufio.NewWriterSize(fp, 1024*1024)
	b.Run("write 1M buf", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err = fpBuf1M.Write(data2K); err != nil {
				b.Fatalf("got error: %+v", err)
			}
			if err = fpBuf1M.Flush(); err != nil {
				b.Fatalf("got error: %+v", err)
			}
		}
	})

	fpBuf4M := bufio.NewWriterSize(fp, 1024*1024*4)
	b.Run("write 4M buf", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err = fpBuf4M.Write(data2K); err != nil {
				b.Fatalf("got error: %+v", err)
			}
			if err = fpBuf4M.Flush(); err != nil {
				b.Fatalf("got error: %+v", err)
			}
		}
	})

}
