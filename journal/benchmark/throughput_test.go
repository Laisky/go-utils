package journal_test

import (
	"io"
	"io/ioutil"
	"log"
	"os"
	"testing"
	"time"

	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/go-utils/journal"
)

func fakedata(m map[string]interface{}) {
	m["data"] = utils.RandomStringWithLength(2048)
}

func BenchmarkData(b *testing.B) {
	dir, err := ioutil.TempDir("", "journal-test")
	if err != nil {
		log.Fatal(err)
	}
	b.Logf("create directory: %v", dir)
	defer os.RemoveAll(dir)

	cfg := &journal.JournalConfig{
		BufDirPath:   dir,
		BufSizeBytes: 314572800,
	}
	j := journal.NewJournal(cfg)

	data := map[string]interface{}{"id": int64(1000)}
	fakedata(data)
	b.Logf("write data: %+v", data)
	b.Run("write", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
		WRITE_DATA:
			if err = j.WriteData(&data); err == journal.DuringRotateErr {
				b.Log("write data waiting to rotate...")
				time.Sleep(1 * time.Millisecond)
				goto WRITE_DATA
			} else if err != nil {
				b.Fatalf("got error: %+v", err)
			}
		}
	})

	if err = j.Flush(); err != nil {
		b.Fatalf("got error: %+v", err)
	}
	if err = j.Rotate(); err != nil {
		b.Fatalf("got error: %+v", err)
	}
	b.Run("read", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if err = j.LoadLegacyBuf(&data); err == io.EOF {
				return
			} else if err != nil {
				b.Fatalf("got error: %+v", err)
			}
		}
	})
}
