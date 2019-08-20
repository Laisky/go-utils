package journal_test

import (
	"io"
	"io/ioutil"
	"log"
	"os"
	"testing"

	"github.com/Laisky/go-utils/journal"
)

func TestLegacy(t *testing.T) {
	for _, isCompress := range [...]bool{true, false} {
		// create data files
		dataFp1, err := ioutil.TempFile("", "journal-test")
		if err != nil {
			t.Fatalf("%+v", err)
		}
		defer dataFp1.Close()
		defer os.Remove(dataFp1.Name())
		t.Logf("create file name: %v", dataFp1.Name())

		dataFp2, err := ioutil.TempFile("", "journal-test")
		if err != nil {
			t.Fatalf("%+v", err)
		}
		defer dataFp2.Close()
		defer os.Remove(dataFp2.Name())
		t.Logf("create file name: %v", dataFp2.Name())

		// create ids files
		idsFp1, err := ioutil.TempFile("", "journal-test")
		if err != nil {
			t.Fatalf("%+v", err)
		}
		defer idsFp1.Close()
		defer os.Remove(idsFp1.Name())
		t.Logf("create file name: %v", idsFp1.Name())

		idsFp2, err := ioutil.TempFile("", "journal-test")
		if err != nil {
			t.Fatalf("%+v", err)
		}
		defer idsFp2.Close()
		defer os.Remove(idsFp2.Name())
		t.Logf("create file name: %v", idsFp2.Name())

		// put data
		dataEncoder, err := journal.NewDataEncoder(dataFp1, isCompress)
		if err != nil {
			t.Fatalf("%+v", err)
		}
		dataEncoder.Write(&journal.Data{Data: map[string]interface{}{"data": "data 1"}, ID: 1})
		dataEncoder.Write(&journal.Data{Data: map[string]interface{}{"data": "data 2"}, ID: 2})
		if err = dataEncoder.Flush(); err != nil {
			t.Fatalf("got error: %+v", err)
		}

		dataEncoder, err = journal.NewDataEncoder(dataFp2, isCompress)
		if err != nil {
			t.Fatalf("%+v", err)
		}
		dataEncoder.Write(&journal.Data{Data: map[string]interface{}{"data": "data 21"}, ID: 21})
		dataEncoder.Write(&journal.Data{Data: map[string]interface{}{"data": "data 22"}, ID: 22})
		if err = dataEncoder.Flush(); err != nil {
			t.Fatalf("got error: %+v", err)
		}

		// put ids
		// except 2
		idsEncoder, err := journal.NewIdsEncoder(idsFp1, isCompress)
		if err != nil {
			t.Fatalf("%+v", err)
		}
		if err = idsEncoder.Write(1); err != nil {
			t.Fatalf("got error: %+v", err)
		}
		if err = idsEncoder.Write(21); err != nil {
			t.Fatalf("got error: %+v", err)
		}
		if err = idsEncoder.Flush(); err != nil {
			t.Fatalf("got error: %+v", err)
		}

		idsEncoder, err = journal.NewIdsEncoder(idsFp2, isCompress)
		if err != nil {
			t.Fatalf("%+v", err)
		}
		if err = idsEncoder.Write(22); err != nil {
			t.Fatalf("got error: %+v", err)
		}
		if err = idsEncoder.Flush(); err != nil {
			t.Fatalf("got error: %+v", err)
		}
		dataFp1.Close()
		dataFp2.Close()
		idsFp1.Close()
		idsFp2.Close()

		legacy := journal.NewLegacyLoader(
			[]string{dataFp1.Name(), dataFp2.Name()},
			[]string{idsFp1.Name(), idsFp2.Name()},
			isCompress,
		)
		idmaps := journal.NewInt64Set()
		err = legacy.LoadAllids(idmaps)
		t.Logf("got ids: %+v", idmaps)
		if err = idsEncoder.Write(22); err != nil {
			t.Fatalf("got error: %+v", err)
		}
		if idmaps.CheckAndRemove(0) {
			t.Fatal("should not contains 0")
		}
		if idmaps.CheckAndRemove(33) {
			t.Fatal("should not contains 33")
		}
		if idmaps.CheckAndRemove(2) {
			t.Fatal("should not contains 2")
		}

		dataIds := []int64{}
		for {
			data := journal.Data{}
			err = legacy.Load(&data)
			if err == io.EOF {
				break
			} else if err != nil {
				t.Fatalf("got error: %+v", err)
			}
			t.Logf("got data[%v]", data.ID)
			dataIds = append(dataIds, data.ID)
		}
		t.Logf("got dataIds: %+v", dataIds)
		for _, id := range dataIds {
			if id != 2 {
				t.Fatal("should equal to 2")
			}
		}
	}
}

func TestEmptyLegacy(t *testing.T) {
	for _, isCompress := range [...]bool{true, false} {
		dir, err := ioutil.TempDir("", "journal-test")
		if err != nil {
			log.Fatal(err)
		}
		t.Logf("create directory: %v", dir)
		defer os.RemoveAll(dir)

		legacy := journal.NewLegacyLoader(
			[]string{},
			[]string{},
			isCompress,
		)
		ids := journal.NewInt64Set()
		err = legacy.LoadAllids(ids)
		if err != nil {
			t.Fatalf("got error: %+v", err)
		}
		t.Logf("load ids: %+v", ids)

		data := journal.Data{}
		for {
			if err = legacy.Load(&data); err == io.EOF {
				return
			} else if err != nil {
				t.Fatalf("got error: %+v", err)
			}
		}
	}
}
