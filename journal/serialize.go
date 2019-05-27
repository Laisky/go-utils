package journal

import (
	"bufio"
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

// DataEncoder data serializer
type DataEncoder struct {
	writeChan chan interface{}
	writer    *msgp.Writer
}

// DataDecoder data deserializer
type DataDecoder struct {
	readChan chan interface{}
	reader   *msgp.Reader
}

// IdsEncoder ids serializer
type IdsEncoder struct {
	baseID int64
	writer *bufio.Writer
}

// IdsDecoder ids deserializer
type IdsDecoder struct {
	baseID int64
	reader *bufio.Reader
}

// NewDataEncoder create new DataEncoder
func NewDataEncoder(fp *os.File) *DataEncoder {
	return &DataEncoder{
		writer: msgp.NewWriterSize(fp, BufSize),
	}
}

// NewIdsEncoder create new IdsEncoder
func NewIdsEncoder(fp *os.File) *IdsEncoder {
	return &IdsEncoder{
		baseID: -1,
		writer: bufio.NewWriter(fp),
	}
}

// NewIdsDecoder create new IdsDecoder
func NewIdsDecoder(fp *os.File) *IdsDecoder {
	return &IdsDecoder{
		baseID: -1,
		reader: bufio.NewReaderSize(fp, BufSize),
	}
}

// NewDataDecoder create new DataDecoder
func NewDataDecoder(fp *os.File) *DataDecoder {
	reader := msgp.NewReaderSize(fp, BufSize)
	return &DataDecoder{
		reader: reader,
	}
}

// Write serialize data info fp
func (enc *DataEncoder) Write(msg *Data) (err error) {
	if err = msg.EncodeMsg(enc.writer); err != nil {
		return errors.Wrap(err, "try to Encode journal data got error")
	}

	// if err = enc.writer.Flush(); err != nil {
	// 	return errors.Wrap(err, "try to flush journal data got error")
	// }
	return nil
}

// Flush flush buf to fp
func (enc *DataEncoder) Flush() error {
	return enc.writer.Flush()
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

	if err = binary.Write(enc.writer, bitOrder, offset); err != nil {
		return errors.Wrap(err, "try to write ids got error")
	}

	// if err = enc.writer.Flush(); err != nil {
	// 	return errors.Wrap(err, "try to flush ids got error")
	// }

	// utils.Logger.Debug("write id", zap.Int64("offset", offset), zap.Int64("id", id))
	return nil
}

// Flush flush buf to fp
func (enc *IdsEncoder) Flush() error {
	return enc.writer.Flush()
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
func (dec *IdsDecoder) ReadAllToInt64Set(ids *Int64Set) (err error) {
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
		ids.Add(id)
	}

	return nil
}
