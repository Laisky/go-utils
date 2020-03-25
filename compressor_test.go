package utils

import (
	"bytes"
	"compress/gzip"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/Laisky/zap"
)

func TestUnzipAndZipFiles(t *testing.T) {
	var err error
	// if err = Logger.ChangeLevel("debug"); err != nil {
	// 	t.Fatalf("%+v", err)
	// }

	var dir string
	if dir, err = ioutil.TempDir("", "compressor-test"); err != nil {
		log.Fatal(err)
	}
	t.Logf("create directory: %v", dir)
	defer os.RemoveAll(dir)

	if err = os.Mkdir(filepath.Join(dir, "src"), os.ModePerm); err != nil {
		t.Fatalf("%+v", err)
	}
	if err = os.Mkdir(filepath.Join(dir, "dst"), os.ModePerm); err != nil {
		t.Fatalf("%+v", err)
	}
	files := []string{
		filepath.Join(dir, "src", "a.txt"),
		filepath.Join(dir, "src", "b.txt"),
		filepath.Join(dir, "src", "c.txt"),
	}

	var fp *os.File
	for _, file := range files {
		if fp, err = os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.ModePerm); err != nil {
			t.Fatalf("%+v", err)
		}
		if _, err = fp.WriteString("yoo"); err != nil {
			t.Fatalf("%+v", err)
		}
		if err = fp.Close(); err != nil {
			t.Fatalf("%+v", err)
		}
	}

	if err = ZipFiles(filepath.Join(dir, "src.zip"), files); err != nil {
		t.Fatalf("%+v", err)
	}

	var dstFiles []string
	if dstFiles, err = Unzip(filepath.Join(dir, "src.zip"), filepath.Join(dir, "dst")); err != nil {
		t.Fatalf("%+v", err)
	}
	t.Logf("unzip files: %+v", dstFiles)

	// t.Error()
}

func TestGZCompressor(t *testing.T) {
	originText := "fj2f32f9jp9wsif0weif20if320fi23if"
	writer := &bytes.Buffer{}
	c, err := NewGZCompressor(writer)
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

func ExampleGZCompressor() {
	originText := "fj2f32f9jp9wsif0weif20if320fi23if"
	writer := &bytes.Buffer{}

	var err error
	// writer
	c, err := NewGZCompressor(
		writer,
		WithCompressLevel(defaultGzCompressLevel),           // default
		WithCompressBufSizeByte(defaultCompressBufSizeByte), // default
	)
	if err != nil {
		Logger.Error("new compressor", zap.Error(err))
		return
	}
	if _, err = c.WriteString(originText); err != nil {
		Logger.Error("write string to compressor", zap.Error(err))
		return
	}
	if err = c.Flush(); err != nil {
		Logger.Error("flush compressor", zap.Error(err))
		return
	}

	// reader
	var gz *gzip.Reader
	if gz, err = gzip.NewReader(writer); err != nil {
		Logger.Error("new compressor", zap.Error(err))
		return
	}

	var bs []byte
	if bs, err = ioutil.ReadAll(gz); err != nil {
		Logger.Error("read from compressor", zap.Error(err))
		return
	}

	got := string(bs)
	if got != originText {
		Logger.Error("extract compressed text invalidate",
			zap.String("got", got),
			zap.ByteString("expect", bs))
		return
	}
}

func TestPGZCompressor(t *testing.T) {
	originText := "fj2f32f9jp9wsif0weif20if320fi23if"
	writer := &bytes.Buffer{}
	c, err := NewPGZCompressor(
		writer,
		WithCompressLevel(defaultPGzCompressLevel),          // default
		WithCompressBufSizeByte(defaultCompressBufSizeByte), // default

		WithPGzipBlockSize(defaultPgzCompressBlockSize), // default
		WithPGzipNBlocks(defaultPgzCompressNBlock),      // default
	)
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

func ExamplePGZCompressor() {
	originText := "fj2f32f9jp9wsif0weif20if320fi23if"
	writer := &bytes.Buffer{}

	var err error
	// writer
	c, err := NewPGZCompressor(writer)
	if err != nil {
		Logger.Error("new compressor", zap.Error(err))
		return
	}
	if _, err = c.WriteString(originText); err != nil {
		Logger.Error("write string to compressor", zap.Error(err))
		return
	}
	if err = c.Flush(); err != nil {
		Logger.Error("flush compressor", zap.Error(err))
		return
	}

	// reader
	var gz *gzip.Reader
	if gz, err = gzip.NewReader(writer); err != nil {
		Logger.Error("new compressor", zap.Error(err))
		return
	}

	var bs []byte
	if bs, err = ioutil.ReadAll(gz); err != nil {
		Logger.Error("read from compressor", zap.Error(err))
		return
	}

	got := string(bs)
	if got != originText {
		Logger.Error("extract compressed text invalidate",
			zap.String("got", got),
			zap.ByteString("expect", bs))
		return
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

	payload1K := []byte(RandomStringWithLength(1024))
	payload10K := []byte(RandomStringWithLength(10240))
	payload50K := []byte(RandomStringWithLength(10240 * 5))
	payload100K := []byte(RandomStringWithLength(102400))
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

/*
goos: darwin
goarch: amd64
pkg: github.com/Laisky/go-utils
BenchmarkCompressor/pgzCompressor-blocks4-250000_gz_write_10K-4         	   10195	    115989 ns/op	  584588 B/op	      11 allocs/op
BenchmarkCompressor/pgzCompressor-blocks4-500000_gz_write_10K-4         	   10000	    114282 ns/op	  582810 B/op	      11 allocs/op
BenchmarkCompressor/gzCompressor_gz_write_10K-4                         	    4320	    281743 ns/op	       0 B/op	       0 allocs/op
BenchmarkCompressor/pgzCompressor-blocks2-250000_gz_write_10K-4         	    9741	    115122 ns/op	  581498 B/op	      11 allocs/op
BenchmarkCompressor/pgzCompressor-blocks2-500000_gz_write_10K-4         	   10000	    110754 ns/op	  579144 B/op	      11 allocs/op
BenchmarkCompressor/normal_write_10K-4                                  	 8525589	       139 ns/op	       0 B/op	       0 allocs/op
BenchmarkCompressor/gzCompressor_gz_write_50K-4                         	     571	   1910871 ns/op	       0 B/op	       0 allocs/op
BenchmarkCompressor/pgzCompressor-blocks2-250000_gz_write_50K-4         	    8658	    157012 ns/op	  579934 B/op	      11 allocs/op
BenchmarkCompressor/pgzCompressor-blocks2-500000_gz_write_50K-4         	    7998	    152650 ns/op	  578246 B/op	      11 allocs/op
BenchmarkCompressor/pgzCompressor-blocks4-250000_gz_write_50K-4         	    8598	    152514 ns/op	  577482 B/op	      11 allocs/op
BenchmarkCompressor/pgzCompressor-blocks4-500000_gz_write_50K-4         	    6781	    154517 ns/op	  576757 B/op	      11 allocs/op
BenchmarkCompressor/normal_write_50K-4                                  	  507096	      2544 ns/op	       0 B/op	       0 allocs/op
BenchmarkCompressor/pgzCompressor-blocks4-500000_gz_write_100K-4        	    6258	    222641 ns/op	  578490 B/op	      11 allocs/op
BenchmarkCompressor/gzCompressor_gz_write_100K-4                        	     360	   3366950 ns/op	       0 B/op	       0 allocs/op
BenchmarkCompressor/pgzCompressor-blocks2-250000_gz_write_100K-4        	    6814	    186113 ns/op	  575306 B/op	      11 allocs/op
BenchmarkCompressor/pgzCompressor-blocks2-500000_gz_write_100K-4        	    6603	    190075 ns/op	  570878 B/op	      11 allocs/op
BenchmarkCompressor/pgzCompressor-blocks4-250000_gz_write_100K-4        	    6446	    189761 ns/op	  575713 B/op	      11 allocs/op
BenchmarkCompressor/normal_write_100K-4                                 	  259213	      4791 ns/op	       0 B/op	       0 allocs/op
BenchmarkCompressor/gzCompressor_gz_write_1K-4                          	   17718	     65976 ns/op	       0 B/op	       0 allocs/op
BenchmarkCompressor/pgzCompressor-blocks2-250000_gz_write_1K-4          	   12034	     99579 ns/op	  576201 B/op	      11 allocs/op
BenchmarkCompressor/pgzCompressor-blocks2-500000_gz_write_1K-4          	   10000	    102135 ns/op	  579617 B/op	      11 allocs/op
BenchmarkCompressor/pgzCompressor-blocks4-250000_gz_write_1K-4          	   10000	    100201 ns/op	  577761 B/op	      11 allocs/op
BenchmarkCompressor/pgzCompressor-blocks4-500000_gz_write_1K-4          	   12032	    100153 ns/op	  580680 B/op	      11 allocs/op
BenchmarkCompressor/normal_write_1K-4                                   	41966409	        30.5 ns/op	       0 B/op	       0 allocs/op
PASS
ok  	github.com/Laisky/go-utils	33.127s
Success: Benchmarks passed.
*/
func BenchmarkCompressor(b *testing.B) {
	fp, err := ioutil.TempFile("", "gz-test")
	if err != nil {
		b.Fatalf("%+v", err)
	}
	defer fp.Close()
	defer os.Remove(fp.Name())
	b.Logf("create file name: %v", fp.Name())

	buf := &bytes.Buffer{}
	gzWriter, err := NewGZCompressor(buf)
	if err != nil {
		b.Fatalf("%+v", err)
	}
	pgzWriterP2Size250000, err := NewPGZCompressor(buf, WithPGzipNBlocks(2), WithPGzipBlockSize(250000))
	if err != nil {
		b.Fatalf("%+v", err)
	}
	pgzWriterP2Size500000, err := NewPGZCompressor(buf, WithPGzipNBlocks(2), WithPGzipBlockSize(500000))
	if err != nil {
		b.Fatalf("%+v", err)
	}
	pgzWriterP4Size250000, err := NewPGZCompressor(buf, WithPGzipNBlocks(4), WithPGzipBlockSize(250000))
	if err != nil {
		b.Fatalf("%+v", err)
	}
	pgzWriterP4Size500000, err := NewPGZCompressor(buf, WithPGzipNBlocks(4), WithPGzipBlockSize(500000))
	if err != nil {
		b.Fatalf("%+v", err)
	}

	for pname, payload := range map[string][]byte{
		"1K":   []byte(RandomStringWithLength(1024)),
		"10K":  []byte(RandomStringWithLength(10240)),
		"50K":  []byte(RandomStringWithLength(10240 * 5)),
		"100K": []byte(RandomStringWithLength(102400)),
	} {
		for name, compressWriter := range map[string]CompressorItf{
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
