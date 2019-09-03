package journal

/*
gzWriter -> writer -> fp
fp -> gzReader -> reader
*/

import (
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sync"

	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/zap"
	"github.com/RoaringBitmap/roaring"
	"github.com/pkg/errors"
	"github.com/tinylib/msgp/msgp"
)

var (
	once     = &sync.Once{}
	bitOrder = binary.BigEndian
)

// BaseSerializer base serializer
type BaseSerializer struct {
	sync.Mutex
	isCompress bool
}

// DataEncoder data serializer
type DataEncoder struct {
	BaseSerializer
	writeChan chan interface{}
	writer    *msgp.Writer
	gzWriter  *utils.GZCompressor
}

// DataDecoder data deserializer
type DataDecoder struct {
	BaseSerializer
	readChan chan interface{}
	reader   *msgp.Reader
	gzReader io.Reader
}

// IdsEncoder ids serializer
type IdsEncoder struct {
	BaseSerializer
	baseID   int64
	writer   *bufio.Writer
	gzWriter *utils.GZCompressor
}

// IdsDecoder ids deserializer
type IdsDecoder struct {
	BaseSerializer
	baseID   int64
	reader   *bufio.Reader
	gzReader io.Reader
}

// NewDataEncoder create new DataEncoder
func NewDataEncoder(fp *os.File, isCompress bool) (enc *DataEncoder, err error) {
	enc = &DataEncoder{
		BaseSerializer: BaseSerializer{
			isCompress: isCompress,
		},
	}
	if isCompress {
		if enc.gzWriter, err = utils.NewGZCompressor(&utils.GZCompressorCfg{
			BufSizeByte: BufSize,
			Writer:      fp,
			GzLevel:     gzip.BestSpeed,
		}); err != nil {
			return nil, err
		}
		enc.writer = msgp.NewWriterSize(enc.gzWriter, BufSize)
	} else {
		enc.writer = msgp.NewWriterSize(fp, BufSize)
	}
	return enc, nil
}

// NewIdsEncoder create new IdsEncoder
func NewIdsEncoder(fp *os.File, isCompress bool) (enc *IdsEncoder, err error) {
	enc = &IdsEncoder{
		BaseSerializer: BaseSerializer{
			isCompress: isCompress,
		},
		baseID: -1,
	}
	if isCompress {
		if enc.gzWriter, err = utils.NewGZCompressor(&utils.GZCompressorCfg{
			BufSizeByte: BufSize,
			Writer:      fp,
			GzLevel:     gzip.BestSpeed,
		}); err != nil {
			return nil, err
		}
		enc.writer = bufio.NewWriterSize(enc.gzWriter, BufSize)
	} else {
		enc.writer = bufio.NewWriterSize(fp, BufSize)
	}
	return enc, nil
}

// NewIdsDecoder create new IdsDecoder
func NewIdsDecoder(fp *os.File, isCompress bool) (decoder *IdsDecoder, err error) {
	decoder = &IdsDecoder{
		BaseSerializer: BaseSerializer{
			isCompress: isCompress,
		},
		baseID: -1,
	}
	if isCompress {
		decoder.gzReader, err = gzip.NewReader(fp)
		if err != nil {
			return nil, errors.Wrap(err, "try to use gzip read ids fp got error")
		}
		decoder.reader = bufio.NewReaderSize(decoder.gzReader, BufSize)
	} else {
		decoder.reader = bufio.NewReaderSize(fp, BufSize)
	}

	return decoder, nil
}

// NewDataDecoder create new DataDecoder
func NewDataDecoder(fp *os.File, isCompress bool) (decoder *DataDecoder, err error) {
	decoder = &DataDecoder{
		BaseSerializer: BaseSerializer{
			isCompress: isCompress,
		},
	}
	if isCompress {
		decoder.gzReader, err = gzip.NewReader(fp)
		if err != nil {
			return nil, errors.Wrap(err, "try to use gzip read ids fp got error")
		}
		decoder.reader = msgp.NewReaderSize(decoder.gzReader, BufSize)
	} else {
		decoder.reader = msgp.NewReaderSize(fp, BufSize)
	}
	return decoder, err
}

// Write serialize data info fp
func (enc *DataEncoder) Write(msg *Data) (err error) {
	enc.Lock()
	defer enc.Unlock()
	if err = msg.EncodeMsg(enc.writer); err != nil {
		return errors.Wrap(err, "try to Encode journal data got error")
	}
	enc.writer.Flush()
	if enc.isCompress {
		enc.gzWriter.WriteFooter()
	}

	return nil
}

// Flush flush buf to fp
func (enc *DataEncoder) Flush() (err error) {
	enc.Lock()
	defer enc.Unlock()
	if err = enc.writer.Flush(); err != nil {
		return errors.Wrap(err, "try to flush data encoder got error")
	}
	if enc.isCompress {
		if err = enc.gzWriter.Flush(); err != nil {
			return errors.Wrap(err, "try to flush data encoder gz got error")
		}
	}
	return
}

// Close close data gzip writer
func (enc *DataEncoder) Close() (err error) {
	enc.Lock()
	defer enc.Unlock()
	if err = enc.writer.Flush(); err != nil {
		return errors.Wrap(err, "try to flush data encoder got error")
	}
	if enc.isCompress {
		if err = enc.gzWriter.Flush(); err != nil {
			return errors.Wrap(err, "try to close data gz encoder got error")
		}
	}
	enc.writer = nil
	return
}

// Read deserialize data from fp
func (dec *DataDecoder) Read(data *Data) (err error) {
	if err = data.DecodeMsg(dec.reader); err == msgp.WrapError(io.EOF) {
		return io.EOF
	} else if err != nil {
		return err
	}

	return nil
}

// Write serialize id info fp
func (enc *IdsEncoder) Write(id int64) (err error) {
	if id < 0 {
		return fmt.Errorf("id should bigger than 0, but got `%v`", id)
	}

	var offset int64
	if enc.baseID == -1 {
		enc.baseID = id
		offset = id // set first id as baseID
		utils.Logger.Debug("set write base id", zap.Int64("baseID", id))
	} else {
		offset = id - enc.baseID // offset
	}

	enc.Lock()
	defer enc.Unlock()
	if err = binary.Write(enc.writer, bitOrder, offset); err != nil {
		return errors.Wrap(err, "try to write ids got error")
	}
	enc.writer.Flush()
	if enc.isCompress {
		enc.gzWriter.WriteFooter()
	}

	// utils.Logger.Debug("write id", zap.Int64("offset", offset), zap.Int64("id", id))
	return nil
}

// Flush flush buf to fp
func (enc *IdsEncoder) Flush() (err error) {
	enc.Lock()
	defer enc.Unlock()
	if err = enc.writer.Flush(); err != nil {
		return errors.Wrap(err, "try to flush ids encoder got error")
	}
	if enc.isCompress {
		if err = enc.gzWriter.Flush(); err != nil {
			return errors.Wrap(err, "try to flush ids encoder gz got error")
		}
	}

	return
}

// Close close ids gzip writer
func (enc *IdsEncoder) Close() (err error) {
	enc.Lock()
	defer enc.Unlock()
	if err = enc.writer.Flush(); err != nil {
		return errors.Wrap(err, "try to flush ids encoder got error")
	}
	if enc.isCompress {
		if err = enc.gzWriter.Flush(); err != nil {
			return errors.Wrap(err, "try to close ids gz encoder got error")
		}
	}
	enc.writer = nil
	return
}

// LoadMaxId load the maxium id in all files
func (dec *IdsDecoder) LoadMaxId() (maxId int64, err error) {
	var id int64
	for {
		if err = binary.Read(dec.reader, bitOrder, &id); err == io.EOF {
			break
		} else if err != nil {
			return 0, errors.Wrap(err, "try to read ids got error")
		}

		if dec.baseID == -1 {
			utils.Logger.Debug("set baseID", zap.Int64("id", id))
			dec.baseID = id
		} else {
			id += dec.baseID
		}

		// utils.Logger.Debug("load new id", zap.Int64("id", id))
		if id > maxId {
			maxId = id
		}
	}

	return maxId, nil
}

// ReadAllToBmap read all ids in all files into bmap
func (dec *IdsDecoder) ReadAllToBmap() (ids *roaring.Bitmap, err error) {
	bitmap := roaring.New()
	var id int64
	for {
		if err = binary.Read(dec.reader, bitOrder, &id); err == io.EOF {
			break
		} else if err != nil {
			return nil, errors.Wrap(err, "try to read ids got error")
		}

		if dec.baseID == -1 {
			// first id in head of file is baseID
			utils.Logger.Debug("set baseID", zap.Int64("id", id))
			dec.baseID = id
		} else {
			// another ids in rest file are offsets
			id += dec.baseID
		}

		// utils.Logger.Debug("load new id", zap.Int64("id", id))
		bitmap.AddInt(int(id))
	}

	return bitmap, nil
}

// ReadAllToBmap read all ids in all files into set
func (dec *IdsDecoder) ReadAllToInt64Set(ids Int64SetItf) (err error) {
	var id int64
	for {
		if err = binary.Read(dec.reader, bitOrder, &id); err == io.EOF {
			break
		} else if err != nil {
			return errors.Wrap(err, "try to read ids got error")
		}

		if dec.baseID == -1 {
			// first id in head of file is baseID
			utils.Logger.Debug("set baseID", zap.Int64("id", id))
			dec.baseID = id
		} else {
			// another ids in rest file are offsets
			id += dec.baseID
		}

		// utils.Logger.Debug("load new id", zap.Int64("id", id))
		ids.AddInt64(id)
	}

	return nil
}
