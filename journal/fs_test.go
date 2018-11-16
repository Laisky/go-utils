package journal_test

import (
	"io/ioutil"
	"log"
	"os"
	"testing"
	"time"

	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/go-utils/journal"
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
		newFName, err = journal.GenerateNewBufFName(now, testcase.OldFName)
		if err != nil {
			t.Fatalf("got error: %+v", err)
		}
		if newFName != testcase.ExpectFName {
			t.Errorf("expect %v, got %v", testcase.ExpectFName, newFName)
		}
	}
}

func TestPrepareNewBufFile(t *testing.T) {
	dir, err := ioutil.TempDir("", "golang-test")
	if err != nil {
		log.Fatal(err)
	}
	t.Logf("create directory: %v", dir)
	defer os.RemoveAll(dir)

	bufStat, err := journal.PrepareNewBufFile(dir, nil, true)
	if err != nil {
		t.Fatalf("got error: %+v", err)
	}
	defer bufStat.NewDataFp.Close()
	defer bufStat.NewIdsDataFp.Close()

	_, err = bufStat.NewDataFp.WriteString("test data")
	if err != nil {
		t.Fatalf("%+v", err)
	}
	_, err = bufStat.NewIdsDataFp.WriteString("test ids")
	if err != nil {
		t.Fatalf("%+v", err)
	}

	err = bufStat.NewDataFp.Sync()
	if err != nil {
		t.Fatalf("%+v", err)
	}
	err = bufStat.NewIdsDataFp.Sync()
	if err != nil {
		t.Fatalf("%+v", err)
	}
}

func init() {
	utils.SetupLogger("debug")
}
