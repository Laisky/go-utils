package journal_test

import (
	"io"
	"io/ioutil"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/Laisky/go-utils/journal"
)

func TestSerializer(t *testing.T) {
	fp, err := ioutil.TempFile("", "journal-test")
	if err != nil {
		t.Fatalf("%+v", err)
	}
	defer fp.Close()
	defer os.Remove(fp.Name())
	t.Logf("create file name: %v", fp.Name())

	m := map[string]interface{}{"tag": "testtag", "message": 123}

	encoder := journal.NewDataEncoder(fp)
	if err = encoder.Write(&m); err != nil {
		t.Fatalf("%+v", err)
	}
	if err = fp.Sync(); err != nil {
		t.Fatalf("%+v", err)
	}

	var got = map[string]interface{}{}
	fp.Seek(0, 0)
	decoder := journal.NewDataDecoder(fp)
	if err = decoder.Read(&got); err != nil {
		t.Fatalf("%+v", err)
	}

	t.Logf("got: %+v", got)
	if string(got["tag"].([]byte)) != m["tag"] ||
		int(got["message"].(int64)) != m["message"] {
		t.Errorf("expect %v:%v, got %v:%v", m["tag"], m["message"], string(got["tag"].([]byte)), int(got["message"].(int64)))
	}
}

func BenchmarkSerializer(b *testing.B) {
	fp, err := ioutil.TempFile("", "journal-test")
	if err != nil {
		b.Fatalf("%+v", err)
	}
	defer fp.Close()
	defer os.Remove(fp.Name())
	b.Logf("create file name: %v", fp.Name())
	m := map[string]interface{}{"tag": "tag", "message": "jr32oirj23r2ifj32ofjo2jfo2fjof2"}
	encoder := journal.NewDataEncoder(fp)

	b.Run("encoder", func(b *testing.B) {
		for i := 0; i < 100000; i++ {
			if err = encoder.Write(&m); err != nil {
				b.Fatalf("%+v", err)
			}
		}
		fp.Sync()
	})

	fp.Seek(0, 0)
	decoder := journal.NewDataDecoder(fp)
	b.Run("decoder", func(b *testing.B) {
		v := map[string]interface{}{}
		for {
			if err = decoder.Read(&v); err == io.EOF {
				return
			} else if err != nil {
				b.Fatalf("%+v", err)
			}

			if string(v["tag"].([]byte)) != "tag" {
				b.Error("tag incorrect")
			}
		}
	})
}

func TestIdsSerializer(t *testing.T) {
	fp, err := ioutil.TempFile("", "journal-test")
	if err != nil {
		t.Fatalf("%+v", err)
	}
	defer fp.Close()
	defer os.Remove(fp.Name())
	t.Logf("create file name: %v", fp.Name())

	encoder := journal.NewIdsEncoder(fp)
	decoder := journal.NewIdsDecoder(fp)

	for id := int64(0); id < 1000; id++ {
		err = encoder.Write(id)
		if err != nil {
			t.Fatalf("%+v", err)
		}

		err = encoder.Write(math.MaxInt64 + id + 100)
		if err != nil {
			if !strings.Contains(err.Error(), "id should bigger than 0") {
				t.Fatalf("%+v", err)
			}
		}
	}

	fp.Seek(0, 0)
	ids, err := decoder.ReadAllToBmap()
	if err != nil {
		t.Fatalf("%+v", err)
	}

	t.Logf("got ids: %+v", ids)
	for id := 0; id < 2000; id++ {
		if id < 1000 && !ids.ContainsInt(id) {
			t.Fatalf("%v should in ids", id)
		}
		if id >= 1000 && ids.ContainsInt(id) {
			t.Fatalf("%v should not in ids", id)
		}
	}
}
