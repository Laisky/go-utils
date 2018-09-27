package journal

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"reflect"
	"sync"

	utils "github.com/Laisky/go-utils"
	"github.com/RoaringBitmap/roaring"
	"github.com/pkg/errors"
	"github.com/ugorji/go/codec"
	"go.uber.org/zap"
)

var (
	once     = &sync.Once{}
	bitOrder = binary.BigEndian
)

type DataEncoder struct {
	encoder   *codec.Encoder
	writeChan chan interface{}
	writer    *bufio.Writer
}

type DataDecoder struct {
	decoder  *codec.Decoder
	readChan chan interface{}
	reader   *bufio.Reader
}

type IdsEncoder struct {
	baseId int64
	writer *bufio.Writer
}

type IdsDecoder struct {
	baseId int64
	reader *bufio.Reader
}

func NewCodec() *codec.MsgpackHandle {
	_codec := &codec.MsgpackHandle{}
	_codec.RawToString = false
	_codec.MapType = reflect.TypeOf(map[string]interface{}(nil))
	_codec.DecodeOptions.MapValueReset = true
	return _codec
}

func NewDataEncoder(fp *os.File) *DataEncoder {
	writer := bufio.NewWriterSize(fp, BufSize)
	return &DataEncoder{
		writer:  writer,
		encoder: codec.NewEncoder(writer, NewCodec()),
	}
}

func NewIdsEncoder(fp *os.File) *IdsEncoder {
	return &IdsEncoder{
		baseId: -1,
		writer: bufio.NewWriter(fp),
	}
}

func NewIdsDecoder(fp *os.File) *IdsDecoder {
	return &IdsDecoder{
		baseId: -1,
		reader: bufio.NewReaderSize(fp, BufSize),
	}
}

func NewDataDecoder(fp *os.File) *DataDecoder {
	reader := bufio.NewReader(fp)
	return &DataDecoder{
		reader:  reader,
		decoder: codec.NewDecoder(reader, NewCodec()),
	}
}

func (enc *DataEncoder) Write(msg *map[string]interface{}) (err error) {
	if err = enc.encoder.Encode(msg); err != nil {
		return errors.Wrap(err, "try to Encode journal data got error")
	}

	// if err = enc.writer.Flush(); err != nil {
	// 	return errors.Wrap(err, "try to flush journal data got error")
	// }
	return nil
}

func (enc *DataEncoder) Flush() error {
	return enc.writer.Flush()
}

func (dec *DataDecoder) Read(v *map[string]interface{}) error {
	return dec.decoder.Decode(v)
}

func (enc *IdsEncoder) Write(id int64) (err error) {
	if id < 0 {
		return fmt.Errorf("id should bigger than 0, but got `%v`", id)
	}

	var offset int64
	if enc.baseId == -1 {
		enc.baseId = id
		offset = id
		utils.Logger.Debug("set write base id", zap.Int64("baseid", id))
	} else {
		offset = id - enc.baseId // offset
	}

	if err = binary.Write(enc.writer, bitOrder, offset); err != nil {
		return errors.Wrap(err, "try to write ids got error")
	}

	// if err = enc.writer.Flush(); err != nil {
	// 	return errors.Wrap(err, "try to flush ids got error")
	// }

	utils.Logger.Debug("write id", zap.Int64("offset", offset), zap.Int64("id", id))
	return nil
}

func (enc *IdsEncoder) Flush() error {
	return enc.writer.Flush()
}

func (dec *IdsDecoder) LoadMaxId() (maxId int64, err error) {
	var id int64
	for {
		if err = binary.Read(dec.reader, bitOrder, &id); err == io.EOF {
			break
		} else if err != nil {
			return 0, errors.Wrap(err, "try to read ids got error")
		}

		if dec.baseId == -1 {
			utils.Logger.Debug("set baseid", zap.Int64("id", id))
			dec.baseId = id
		} else {
			id += dec.baseId
		}

		utils.Logger.Debug("load new id", zap.Int64("id", id))
		if id > maxId {
			maxId = id
		}
	}

	return maxId, nil
}

func (dec *IdsDecoder) ReadAllToBmap() (ids *roaring.Bitmap, err error) {
	bitmap := roaring.New()
	var id int64
	for {
		if err = binary.Read(dec.reader, bitOrder, &id); err == io.EOF {
			break
		} else if err != nil {
			return nil, errors.Wrap(err, "try to read ids got error")
		}

		if dec.baseId == -1 {
			utils.Logger.Debug("set baseid", zap.Int64("id", id))
			dec.baseId = id
		} else {
			id += dec.baseId
		}

		utils.Logger.Debug("load new id", zap.Int64("id", id))
		bitmap.AddInt(int(id))
	}

	return bitmap, nil
}

func GetId(data map[string]interface{}) int64 {
	return data["id"].(int64)
}
