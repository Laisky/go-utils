package journal_test

import (
	"io/ioutil"
	"log"
	"os"
	"testing"
	"time"

	"github.com/ncw/directio"

	"github.com/coreos/etcd/pkg/fileutil"

	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/go-utils/journal"
)

const (
	defaultBufFileSizeBytes = 1000000
)

type FNameCase struct {
	OldFName, ExpectFName, NowTS string
}

func TestGenerateNewBufFName(t *testing.T) {
	var (
		err      error
		now      time.Time
		newFName string
		cases    = []*FNameCase{
			&FNameCase{
				OldFName:    "20060102_00000001.buf",
				ExpectFName: "20060102_00000002.buf",
				NowTS:       "20060102-0700",
			},
			&FNameCase{
				OldFName:    "20060102_00000001.ids",
				ExpectFName: "20060102_00000002.ids",
				NowTS:       "20060102-0700",
			},
			&FNameCase{
				OldFName:    "20060102_00000002.buf",
				ExpectFName: "20060104_00000001.buf",
				NowTS:       "20060104-0700",
			},
			&FNameCase{
				OldFName:    "20060102_00000002.buf",
				ExpectFName: "20060103_00000001.buf",
				NowTS:       "20060103-0600",
			},
		}
	)

	for _, testcase := range cases {
		now, err = time.Parse("20060102-0700", testcase.NowTS)
		if err != nil {
			t.Fatalf("got error: %+v", err)
		}
		newFName, err = journal.GenerateNewBufFName(now, testcase.OldFName, false)
		if err != nil {
			t.Fatalf("got error: %+v", err)
		}
		if newFName != testcase.ExpectFName {
			t.Errorf("expect %v, got %v", testcase.ExpectFName, newFName)
		}
	}
}

func TestPrepareNewBufFile(t *testing.T) {
	dir, err := ioutil.TempDir("", "journal-test-fs")
	if err != nil {
		log.Fatal(err)
	}
	t.Logf("create directory: %v", dir)
	defer os.RemoveAll(dir)

	bufStat, err := journal.PrepareNewBufFile(dir, nil, true, false, defaultBufFileSizeBytes)
	if err != nil {
		t.Fatalf("got error: %+v", err)
	}
	defer bufStat.NewDataFp.Close()
	defer bufStat.NewIDsFp.Close()

	_, err = bufStat.NewDataFp.WriteString("test data")
	if err != nil {
		t.Fatalf("%+v", err)
	}
	_, err = bufStat.NewIDsFp.WriteString("test ids")
	if err != nil {
		t.Fatalf("%+v", err)
	}

	err = bufStat.NewDataFp.Sync()
	if err != nil {
		t.Fatalf("%+v", err)
	}
	err = bufStat.NewIDsFp.Sync()
	if err != nil {
		t.Fatalf("%+v", err)
	}
}

const (
	benchmarkFsDir = "/data/fluentd/go-utils/"
	// benchmarkFsDir = "/Users/laisky/Downloads/"
)

func BenchmarkFSPreallocate(b *testing.B) {
	utils.SetupLogger("error")
	// create data files
	dataFp1, err := directio.OpenFile(benchmarkFsDir+"fp1.dat", os.O_RDWR|os.O_CREATE, journal.FileMode)
	// dataFp1, err := ioutil.TempFile("", "journal-test")
	if err != nil {
		b.Fatalf("%+v", err)
	}
	defer dataFp1.Close()
	defer os.Remove(dataFp1.Name())
	b.Logf("create file name: %v", dataFp1.Name())

	dataFp2, err := directio.OpenFile(benchmarkFsDir+"fp2.dat", os.O_RDWR|os.O_CREATE, journal.FileMode)
	if err != nil {
		b.Fatalf("%+v", err)
	}
	defer dataFp2.Close()
	defer os.Remove(dataFp2.Name())
	b.Logf("create file name: %v", dataFp2.Name())

	dataFp3, err := directio.OpenFile(benchmarkFsDir+"fp3.dat", os.O_RDWR|os.O_CREATE, journal.FileMode)
	if err != nil {
		b.Fatalf("%+v", err)
	}
	defer dataFp3.Close()
	defer os.Remove(dataFp3.Name())
	b.Logf("create file name: %v", dataFp3.Name())

	payload := make([]byte, 1024)
	b.Run("normal", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			dataFp1.Write(payload)
			// dataFp1.Sync()
		}
	})

	fileutil.Preallocate(dataFp2, 1024*1024*1000, false)
	b.ResetTimer()
	b.Run("preallocate", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			dataFp2.Write(payload)
			// dataFp2.Sync()
		}
	})

	fileutil.Preallocate(dataFp3, 1024*1024*1000, true)
	b.ResetTimer()
	b.Run("preallocate with extended", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			dataFp3.Write(payload)
			// dataFp3.Sync()
		}
	})

}
