package journal_test

import (
	"bufio"
	"io"
	"io/ioutil"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"

	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/go-utils/journal"
	"github.com/ugorji/go/codec"
)

func TestSerializer(t *testing.T) {
	for _, isCompress := range [...]bool{true, false} {
		fp, err := ioutil.TempFile("", "journal-test")
		if err != nil {
			t.Fatalf("%+v", err)
		}
		defer fp.Close()
		defer os.Remove(fp.Name())
		t.Logf("create file name: %v", fp.Name())

		m := &journal.Data{
			Data: map[string]interface{}{"tag": "testtag", "message": 123},
		}

		encoder, err := journal.NewDataEncoder(fp, isCompress)
		if err != nil {
			t.Fatalf("%+v", err)
		}
		if err = encoder.Write(m); err != nil {
			t.Fatalf("%+v", err)
		}
		if err = encoder.Flush(); err != nil {
			t.Fatalf("%+v", err)
		}

		var got = &journal.Data{}
		if _, err = fp.Seek(0, 0); err != nil {
			t.Fatalf("seek: %+v", err)
		}
		var decoder *journal.DataDecoder
		if decoder, err = journal.NewDataDecoder(fp, isCompress); err != nil {
			t.Fatalf("%+v", err)
		}
		if err = decoder.Read(got); err != nil {
			t.Fatalf("%+v", err)
		}

		t.Logf("got: %+v", got)
		if string(got.Data["tag"].(string)) != m.Data["tag"] ||
			int(got.Data["message"].(int64)) != m.Data["message"] {
			t.Errorf("expect %v:%v, got %v:%v", m.Data["tag"], m.Data["message"], string(got.Data["tag"].(string)), int(got.Data["message"].(int64)))
		}
	}
}

func BenchmarkSerializerWithCompress(b *testing.B) {
	fp, err := ioutil.TempFile("", "journal-test")
	if err != nil {
		b.Fatalf("%+v", err)
	}
	defer fp.Close()
	defer os.Remove(fp.Name())
	b.Logf("create file name: %v", fp.Name())
	m := &journal.Data{
		Data: map[string]interface{}{"tag": "tag", "message": "jr32oirj23r2ifj32ofjfwefefwfwfwefwefwef 234rt34t 34t 34t43t 34t o2jfo2fjof2"},
	}
	encoder, err := journal.NewDataEncoder(fp, true)
	if err != nil {
		b.Fatalf("%+v", err)
	}

	b.Run("data encoder with compress", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if err = encoder.Write(m); err != nil {
				b.Fatalf("%+v", err)
			}
		}
		encoder.Flush()
	})
	encoder.Flush()

	if _, err = fp.Seek(0, 0); err != nil {
		b.Fatalf("seek: %+v", err)
	}
	n := 0
	decoder, err := journal.NewDataDecoder(fp, true)
	if err != nil {
		b.Fatalf("%+v", err)
	}
	b.Run("data decoder with compress", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			n++
			v := &journal.Data{}
			if err = decoder.Read(v); err == io.EOF {
				return
			} else if err != nil {
				b.Fatalf("%+v", err)
			}

			if string(v.Data["tag"].(string)) != m.Data["tag"].(string) ||
				string(v.Data["message"].(string)) != m.Data["message"].(string) {
				b.Fatal("load incorrect")
			}
		}
	})

	// b.Errorf("run: %v", n)
}

func BenchmarkSerializerWithoutCompress(b *testing.B) {
	fp, err := ioutil.TempFile("", "journal-test")
	if err != nil {
		b.Fatalf("%+v", err)
	}
	defer fp.Close()
	defer os.Remove(fp.Name())
	b.Logf("create file name: %v", fp.Name())
	m := &journal.Data{
		Data: map[string]interface{}{"tag": "tag", "message": "jr32oirj23r2ifj32ofjfwefefwfwfwefwefwef 234rt34t 34t 34t43t 34t o2jfo2fjof2"},
	}
	encoder, err := journal.NewDataEncoder(fp, false)
	if err != nil {
		b.Fatalf("%+v", err)
	}

	b.Run("encoder with compress", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if err = encoder.Write(m); err != nil {
				b.Fatalf("%+v", err)
			}
		}
		encoder.Flush()
	})
	encoder.Flush()

	if _, err = fp.Seek(0, 0); err != nil {
		b.Fatalf("seek: %+v", err)
	}
	n := 0
	decoder, err := journal.NewDataDecoder(fp, false)
	if err != nil {
		b.Fatalf("%+v", err)
	}
	b.Run("decoder with compress", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			n++
			v := &journal.Data{}
			if err = decoder.Read(v); err == io.EOF {
				return
			} else if err != nil {
				b.Fatalf("%+v", err)
			}

			if string(v.Data["tag"].(string)) != m.Data["tag"].(string) ||
				string(v.Data["message"].(string)) != m.Data["message"].(string) {
				b.Fatal("load incorrect")
			}
		}
	})

	// b.Errorf("run: %v", n)
}

func TestIdsSerializer(t *testing.T) {
	for _, isCompress := range [...]bool{true, false} {
		fp, err := ioutil.TempFile("", "journal-test")
		if err != nil {
			t.Fatalf("%+v", err)
		}
		defer fp.Close()
		defer os.Remove(fp.Name())
		t.Logf("create file name: %v", fp.Name())

		encoder, err := journal.NewIdsEncoder(fp, isCompress)
		if err != nil {
			t.Fatalf("%+v", err)
		}

		for id := int64(0); id < 1000; id++ {
			if err = encoder.Write(id); err != nil {
				t.Fatalf("%+v", err)
			}

			err = encoder.Write(math.MaxInt64 + id + 100)
			if err != nil {
				if !strings.Contains(err.Error(), "id should bigger than 0") {
					t.Fatalf("%+v", err)
				}
			}
		}

		if err = encoder.Close(); err != nil {
			t.Fatalf("%+v", err)
		}
		// fp.Close()
		// fp, err = os.Open(fp.Name())
		// if err != nil {
		// 	t.Fatalf("%+v", err)
		// }

		fs, err := fp.Stat()
		if err != nil {
			t.Fatalf("%+v", err)
		}
		t.Logf("file size: %v", fs.Size())
		if _, err = fp.Seek(0, 0); err != nil {
			t.Fatalf("seek: %+v", err)
		}
		decoder, err := journal.NewIdsDecoder(fp, isCompress)
		if err != nil {
			t.Fatalf("got error: %+v", err)
		}
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
}

func NewCodec() *codec.MsgpackHandle {
	_codec := &codec.MsgpackHandle{}
	_codec.RawToString = false
	_codec.MapType = reflect.TypeOf(map[string]interface{}(nil))
	_codec.DecodeOptions.MapValueReset = true
	return _codec
}

func TestCodec(t *testing.T) {
	fp, err := ioutil.TempFile("", "journal-test")
	if err != nil {
		t.Fatalf("%+v", err)
	}
	defer fp.Close()
	defer os.Remove(fp.Name())
	t.Logf("create file name: %v", fp.Name())

	encoder := codec.NewEncoder(bufio.NewWriter(fp), NewCodec())

	var (
		data = map[string]interface{}{}
		msg  string
	)
	for i := 0; i < 100; i++ {
		msg = "12345" + utils.RandomStringWithLength(200-i) + "67890"
		data["id"] = i
		data["message"] = map[string]interface{}{"log": msg}
		if err = encoder.Encode(&data); err != nil {
			t.Fatalf("got error: %+v", err)
		}
	}

	if _, err = fp.Seek(0, 0); err != nil {
		t.Fatalf("seek: %+v", err)
	}
	data["message"] = map[string]interface{}{}
	decoder := codec.NewDecoder(bufio.NewReader(fp), NewCodec())
	for {
		if err = decoder.Decode(&data); err == io.EOF {
			t.Log("all done")
			break
		} else if err != nil {
			t.Fatalf("got error: %+v", err)
		}

		t.Log(string(data["message"].(map[string]interface{})["log"].([]byte)))
	}
}
