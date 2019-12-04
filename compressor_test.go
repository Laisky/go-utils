package utils_test

import (
	"bytes"
	"compress/gzip"
	"io/ioutil"
	"os"
	"testing"

	"github.com/Laisky/go-utils"
)

func TestGZCompressor(t *testing.T) {
	originText := "fj2f32f9jp9wsif0weif20if320fi23if"
	writer := &bytes.Buffer{}
	c, err := utils.NewGZCompressor(writer)
	if err != nil {
		t.Fatalf("got error: %+v", err)
	}
	if _, err = c.WriteString(originText); err != nil {
		t.Fatalf("got error: %+v", err)
	}
	if err = c.Flush(); err != nil {
		t.Fatalf("got error: %+v", err)
	}

	var gz *gzip.Reader
	if gz, err = gzip.NewReader(writer); err != nil {
		t.Fatalf("got error: %+v", err)
	}

	if bs, err := ioutil.ReadAll(gz); err != nil {
		t.Fatalf("got error: %+v", err)
	} else {
		got := string(bs)
		if got != originText {
			t.Fatalf("got: %v", got)
		}
	}
}

func TestPGZCompressor(t *testing.T) {
	originText := "fj2f32f9jp9wsif0weif20if320fi23if"
	writer := &bytes.Buffer{}
	c, err := utils.NewPGZCompressor(writer)
	if err != nil {
		t.Fatalf("got error: %+v", err)
	}
	if _, err = c.WriteString(originText); err != nil {
		t.Fatalf("got error: %+v", err)
	}
	if err = c.Flush(); err != nil {
		t.Fatalf("got error: %+v", err)
	}

	var gz *gzip.Reader
	if gz, err = gzip.NewReader(writer); err != nil {
		t.Fatalf("got error: %+v", err)
	}

	if bs, err := ioutil.ReadAll(gz); err != nil {
		t.Fatalf("got error: %+v", err)
	} else {
		got := string(bs)
		if got != originText {
			t.Fatalf("got: %v", got)
		}
	}
}

/*
goos: darwin
goarch: amd64
pkg: github.com/Laisky/go-utils
BenchmarkGZCompressor/gz_write_1kB-4         	   18213	     64964 ns/op	       0 B/op	       0 allocs/op
BenchmarkGZCompressor/gz_write_10kB-4        	    4683	    290520 ns/op	       0 B/op	       0 allocs/op
BenchmarkGZCompressor/gz_write_50kB-4        	     652	   1593705 ns/op	       0 B/op	       0 allocs/op
BenchmarkGZCompressor/gz_write_100kB-4       	     378	   3050704 ns/op	       0 B/op	       0 allocs/op
BenchmarkGZCompressor/normal_write_1KB-4     	37975290	        29.6 ns/op	       0 B/op	       0 allocs/op
BenchmarkGZCompressor/normal_write_10KB-4    	 8449380	       137 ns/op	       0 B/op	       0 allocs/op
BenchmarkGZCompressor/normal_write_50KB-4    	  531210	      2313 ns/op	       0 B/op	       0 allocs/op
BenchmarkGZCompressor/normal_write_100KB-4   	  247237	      4665 ns/op	       0 B/op	       0 allocs/op
BenchmarkGZCompressor/gz_write_50kB_best_compression-4         	     783	   1491124 ns/op	       0 B/op	       0 allocs/op
BenchmarkGZCompressor/gz_write_50kB_best_speed-4               	    4370	    291897 ns/op	       0 B/op	       0 allocs/op
BenchmarkGZCompressor/gz_write_50kB_HuffmanOnly-4              	    4652	    250891 ns/op	       0 B/op	       0 allocs/op
BenchmarkGZCompressor/normal_write_50KB_to_file-4              	   10000	   4212378 ns/op	       0 B/op	       0 allocs/op
BenchmarkGZCompressor/gz_write_50KB_to_file-4                  	     286	   5067483 ns/op	       0 B/op	       0 allocs/op
BenchmarkGZCompressor/gz_write_50KB_to_file_best_speed-4       	    3494	    412151 ns/op	  148759 B/op	       0 allocs/op
BenchmarkGZCompressor/gz_write_50KB_to_file_BestCompression-4  	     690	   1596123 ns/op	       0 B/op	       0 allocs/op
PASS
ok  	github.com/Laisky/go-utils	61.515s
*/
func BenchmarkGzip(b *testing.B) {
	fp, err := ioutil.TempFile("", "gz-test")
	if err != nil {
		b.Fatalf("%+v", err)
	}
	defer fp.Close()
	defer os.Remove(fp.Name())
	b.Logf("create file name: %v", fp.Name())

	payload1K := []byte(utils.RandomStringWithLength(1024))
	payload10K := []byte(utils.RandomStringWithLength(10240))
	payload50K := []byte(utils.RandomStringWithLength(10240 * 5))
	payload100K := []byte(utils.RandomStringWithLength(102400))
	buf := &bytes.Buffer{}

	gzWriter := gzip.NewWriter(buf)
	b.Run("gz write 1kB", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err = gzWriter.Write(payload1K); err != nil {
				b.Fatalf("write: %+v", err)
			}
			if err = gzWriter.Close(); err != nil {
				b.Fatalf("close: %+v", err)
			}
			gzWriter.Reset(buf)
			buf.Reset()
		}
	})
	buf.Reset()
	b.Run("gz write 10kB", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err = gzWriter.Write(payload10K); err != nil {
				b.Fatalf("write: %+v", err)
			}
			if err = gzWriter.Close(); err != nil {
				b.Fatalf("close: %+v", err)
			}
			gzWriter.Reset(buf)
			buf.Reset()
		}
	})
	buf.Reset()
	b.Run("gz write 50kB", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err = gzWriter.Write(payload50K); err != nil {
				b.Fatalf("write: %+v", err)
			}
			if err = gzWriter.Close(); err != nil {
				b.Fatalf("close: %+v", err)
			}
			gzWriter.Reset(buf)
			buf.Reset()
		}
	})
	buf.Reset()
	b.Run("gz write 100kB", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err = gzWriter.Write(payload100K); err != nil {
				b.Fatalf("write: %+v", err)
			}
			if err = gzWriter.Close(); err != nil {
				b.Fatalf("close: %+v", err)
			}
			gzWriter.Reset(buf)
			buf.Reset()
		}
	})
	buf.Reset()
	b.Run("normal write 1KB", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			buf.Write(payload1K)
			buf.Reset()
		}
	})
	buf.Reset()
	b.Run("normal write 10KB", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			buf.Write(payload10K)
			buf.Reset()
		}
	})
	buf.Reset()
	b.Run("normal write 50KB", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			buf.Write(payload50K)
			buf.Reset()
		}
	})
	buf.Reset()
	b.Run("normal write 100KB", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			buf.Write(payload100K)
			buf.Reset()
		}
	})

	if gzWriter, err = gzip.NewWriterLevel(buf, gzip.BestCompression); err != nil {
		b.Fatalf("got error: %+v", err)
	}
	b.Run("gz write 50kB best compression", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err = gzWriter.Write(payload50K); err != nil {
				b.Fatalf("write: %+v", err)
			}
			if err = gzWriter.Close(); err != nil {
				b.Fatalf("close: %+v", err)
			}
			gzWriter.Reset(buf)
			buf.Reset()
		}
	})
	buf.Reset()
	if gzWriter, err = gzip.NewWriterLevel(buf, gzip.BestSpeed); err != nil {
		b.Fatalf("got error: %+v", err)
	}
	b.Run("gz write 50kB best speed", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err = gzWriter.Write(payload50K); err != nil {
				b.Fatalf("write: %+v", err)
			}
			if err = gzWriter.Close(); err != nil {
				b.Fatalf("close: %+v", err)
			}
			gzWriter.Reset(buf)
			buf.Reset()
		}
	})
	buf.Reset()
	if gzWriter, err = gzip.NewWriterLevel(buf, gzip.HuffmanOnly); err != nil {
		b.Fatalf("got error: %+v", err)
	}
	b.Run("gz write 50kB HuffmanOnly", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err = gzWriter.Write(payload50K); err != nil {
				b.Fatalf("write: %+v", err)
			}
			if err = gzWriter.Close(); err != nil {
				b.Fatalf("close: %+v", err)
			}
			gzWriter.Reset(buf)
			buf.Reset()
		}
	})
	buf.Reset()

	b.Run("normal write 50KB to file", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err = fp.Write(payload50K); err != nil {
				b.Fatalf("write: %+v", err)
			}
		}
	})
	if _, err = fp.Seek(0, 0); err != nil {
		b.Fatalf("seek: %+v", err)
	}

	gzWriter = gzip.NewWriter(fp)
	b.Run("gz write 50KB to file", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err = gzWriter.Write(payload50K); err != nil {
				b.Fatalf("write: %+v", err)
			}
		}
	})
	if _, err = fp.Seek(0, 0); err != nil {
		b.Fatalf("seek: %+v", err)
	}

	if gzWriter, err = gzip.NewWriterLevel(buf, gzip.BestSpeed); err != nil {
		b.Fatalf("got error: %+v", err)
	}
	b.Run("gz write 50KB to file best speed", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err = gzWriter.Write(payload50K); err != nil {
				b.Fatalf("write: %+v", err)
			}
		}
	})
	if _, err = fp.Seek(0, 0); err != nil {
		b.Fatalf("seek: %+v", err)
	}

	if gzWriter, err = gzip.NewWriterLevel(buf, gzip.BestCompression); err != nil {
		b.Fatalf("got error: %+v", err)
	}
	b.Run("gz write 50KB to file BestCompression", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if _, err = gzWriter.Write(payload50K); err != nil {
				b.Fatalf("write: %+v", err)
			}
		}
	})

}
func BenchmarkCompressor(b *testing.B) {
	fp, err := ioutil.TempFile("", "gz-test")
	if err != nil {
		b.Fatalf("%+v", err)
	}
	defer fp.Close()
	defer os.Remove(fp.Name())
	b.Logf("create file name: %v", fp.Name())

	buf := &bytes.Buffer{}
	gzWriter, err := utils.NewGZCompressor(buf)
	if err != nil {
		b.Fatalf("%+v", err)
	}
	pgzWriterP2Size250000, err := utils.NewPGZCompressor(buf, utils.WithPGzipNBlocks(2), utils.WithPGzipBlockSize(250000))
	if err != nil {
		b.Fatalf("%+v", err)
	}
	pgzWriterP2Size500000, err := utils.NewPGZCompressor(buf, utils.WithPGzipNBlocks(2), utils.WithPGzipBlockSize(500000))
	if err != nil {
		b.Fatalf("%+v", err)
	}
	pgzWriterP4Size250000, err := utils.NewPGZCompressor(buf, utils.WithPGzipNBlocks(4), utils.WithPGzipBlockSize(250000))
	if err != nil {
		b.Fatalf("%+v", err)
	}
	pgzWriterP4Size500000, err := utils.NewPGZCompressor(buf, utils.WithPGzipNBlocks(4), utils.WithPGzipBlockSize(500000))
	if err != nil {
		b.Fatalf("%+v", err)
	}

	for pname, payload := range map[string][]byte{
		"1K":   []byte(utils.RandomStringWithLength(1024)),
		"10K":  []byte(utils.RandomStringWithLength(10240)),
		"50K":  []byte(utils.RandomStringWithLength(10240 * 5)),
		"100K": []byte(utils.RandomStringWithLength(102400)),
	} {
		for name, compressWriter := range map[string]utils.CompressorItf{
			"gzCompressor":                 gzWriter,
			"pgzCompressor-blocks2-250000": pgzWriterP2Size250000,
			"pgzCompressor-blocks2-500000": pgzWriterP2Size500000,
			"pgzCompressor-blocks4-250000": pgzWriterP4Size250000,
			"pgzCompressor-blocks4-500000": pgzWriterP4Size500000,
		} {
			b.Run(name+" gz write "+pname, func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					if _, err = compressWriter.Write(payload); err != nil {
						b.Fatalf("write: %+v", err)
					}
					if err = compressWriter.WriteFooter(); err != nil {
						b.Fatalf("close: %+v", err)
					}
					buf.Reset()
				}
			})
			buf.Reset()
		}

		b.Run("normal write "+pname, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				buf.Write(payload)
				buf.Reset()
			}
		})
		buf.Reset()
	}
}
