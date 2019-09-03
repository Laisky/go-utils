package journal_test

import (
	"context"
	"io"
	"io/ioutil"
	"log"
	"os"
	"testing"

	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/go-utils/journal"
)

var (
	ctxKey = utils.CtxKeyT{}
)

func BenchmarkData(b *testing.B) {
	dir, err := ioutil.TempDir("", "journal-test-bench-data")
	if err != nil {
		log.Fatal(err)
	}
	b.Logf("create directory: %v", dir)
	// var err error
	// dir := "/data/go/src/github.com/Laisky/go-utils/journal/benchmark/test"
	defer os.RemoveAll(dir)

	ctx := context.Background()
	cfg := &journal.JournalConfig{
		BufDirPath:   dir,
		BufSizeBytes: 314572800,
	}
	j := journal.NewJournal(
		context.WithValue(ctx, ctxKey, "journal"),
		cfg)

	data := &journal.Data{
		ID:   1000,
		Data: map[string]interface{}{"data": utils.RandomStringWithLength(2048)},
	}
	b.Logf("write data: %+v", data)
	b.Run("write", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if err = j.WriteData(data); err != nil {
				b.Fatalf("got error: %+v", err)
			}
		}
	})

	if err = j.Flush(); err != nil {
		b.Fatalf("got error: %+v", err)
	}
	if err = j.Rotate(context.WithValue(ctx, ctxKey, "rotate")); err != nil {
		b.Fatalf("got error: %+v", err)
	}
	b.Run("read", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			data.ID = 0
			if err = j.LoadLegacyBuf(data); err == io.EOF {
				return
			} else if err != nil {
				b.Fatalf("got error: %+v", err)
			}

			if data.ID != 1000 {
				b.Fatal("read data error")
			}
		}
	})
}
