package journal_test

import (
	"io"
	"io/ioutil"
	"os"
	"testing"

	"github.com/Laisky/go-utils/journal"
)

func TestLegacy(t *testing.T) {
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
	dataEncoder := journal.NewDataEncoder(dataFp1)
	dataEncoder.Write(&map[string]interface{}{"data": "data 1", "id": int64(1)})
	dataEncoder.Write(&map[string]interface{}{"data": "data 2", "id": int64(2)})
	dataEncoder = journal.NewDataEncoder(dataFp2)
	dataEncoder.Write(&map[string]interface{}{"data": "data 21", "id": int64(21)})
	dataEncoder.Write(&map[string]interface{}{"data": "data 22", "id": int64(22)})

	// put ids
	// except 2
	idsEncoder := journal.NewIdsEncoder(idsFp1)
	if err = idsEncoder.Write(1); err != nil {
		t.Fatalf("got error: %+v", err)
	}
	if err = idsEncoder.Write(21); err != nil {
		t.Fatalf("got error: %+v", err)
	}
	idsEncoder = journal.NewIdsEncoder(idsFp2)
	if err = idsEncoder.Write(22); err != nil {
		t.Fatalf("got error: %+v", err)
	}

	legacy := journal.NewLegacyLoader(
		[]string{dataFp1.Name(), dataFp2.Name()},
		[]string{idsFp1.Name(), idsFp2.Name()},
	)
	idmaps, err := legacy.LoadAllids()
	t.Logf("got ids: %+v", idmaps)
	if err = idsEncoder.Write(22); err != nil {
		t.Fatalf("got error: %+v", err)
	}
	if idmaps.ContainsInt(0) {
		t.Fatal("should not contains 0")
	}
	if idmaps.ContainsInt(33) {
		t.Fatal("should not contains 33")
	}
	if idmaps.ContainsInt(2) {
		t.Fatal("should not contains 2")
	}

	dataIds := []int64{}
	for {
		data := map[string]interface{}{}
		err = legacy.Load(&data)
		if err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("got error: %+v", err)
		}
		dataIds = append(dataIds, journal.GetId(data))
	}
	t.Logf("got dataIds: %+v", dataIds)
	for _, id := range dataIds {
		if id != 2 {
			t.Fatal("should equal to 2")
		}
	}

}
