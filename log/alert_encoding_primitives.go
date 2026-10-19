package log

import "time"

// AddBool delegates a valid, bounded primitive using standard Zap encoding.
func (e *alertObjectEncoder) AddBool(key string, v bool) {
	if e.state.text(key) && e.state.take(1) {
		e.ObjectEncoder.AddBool(key, v)
	}
}

// AppendBool delegates a bounded primitive using standard Zap encoding.
func (e *alertArrayEncoder) AppendBool(v bool) {
	if e.state.take(1) {
		e.ArrayEncoder.AppendBool(v)
	}
}

// AddComplex128 delegates a valid, bounded primitive using standard Zap encoding.
func (e *alertObjectEncoder) AddComplex128(key string, v complex128) {
	if e.state.text(key) && e.state.take(1) {
		e.ObjectEncoder.AddComplex128(key, v)
	}
}

// AppendComplex128 delegates a bounded primitive using standard Zap encoding.
func (e *alertArrayEncoder) AppendComplex128(v complex128) {
	if e.state.take(1) {
		e.ArrayEncoder.AppendComplex128(v)
	}
}

// AddComplex64 delegates a valid, bounded primitive using standard Zap encoding.
func (e *alertObjectEncoder) AddComplex64(key string, v complex64) {
	if e.state.text(key) && e.state.take(1) {
		e.ObjectEncoder.AddComplex64(key, v)
	}
}

// AppendComplex64 delegates a bounded primitive using standard Zap encoding.
func (e *alertArrayEncoder) AppendComplex64(v complex64) {
	if e.state.take(1) {
		e.ArrayEncoder.AppendComplex64(v)
	}
}

// AddDuration delegates a valid, bounded primitive using standard Zap encoding.
func (e *alertObjectEncoder) AddDuration(key string, v time.Duration) {
	if e.state.text(key) && e.state.take(1) {
		e.ObjectEncoder.AddDuration(key, v)
	}
}

// AppendDuration delegates a bounded primitive using standard Zap encoding.
func (e *alertArrayEncoder) AppendDuration(v time.Duration) {
	if e.state.take(1) {
		e.ArrayEncoder.AppendDuration(v)
	}
}

// AddFloat64 delegates a valid, bounded primitive using standard Zap encoding.
func (e *alertObjectEncoder) AddFloat64(key string, v float64) {
	if e.state.text(key) && e.state.take(1) {
		e.ObjectEncoder.AddFloat64(key, v)
	}
}

// AppendFloat64 delegates a bounded primitive using standard Zap encoding.
func (e *alertArrayEncoder) AppendFloat64(v float64) {
	if e.state.take(1) {
		e.ArrayEncoder.AppendFloat64(v)
	}
}

// AddFloat32 delegates a valid, bounded primitive using standard Zap encoding.
func (e *alertObjectEncoder) AddFloat32(key string, v float32) {
	if e.state.text(key) && e.state.take(1) {
		e.ObjectEncoder.AddFloat32(key, v)
	}
}

// AppendFloat32 delegates a bounded primitive using standard Zap encoding.
func (e *alertArrayEncoder) AppendFloat32(v float32) {
	if e.state.take(1) {
		e.ArrayEncoder.AppendFloat32(v)
	}
}

// AddInt delegates a valid, bounded primitive using standard Zap encoding.
func (e *alertObjectEncoder) AddInt(key string, v int) {
	if e.state.text(key) && e.state.take(1) {
		e.ObjectEncoder.AddInt(key, v)
	}
}

// AppendInt delegates a bounded primitive using standard Zap encoding.
func (e *alertArrayEncoder) AppendInt(v int) {
	if e.state.take(1) {
		e.ArrayEncoder.AppendInt(v)
	}
}

// AddInt64 delegates a valid, bounded primitive using standard Zap encoding.
func (e *alertObjectEncoder) AddInt64(key string, v int64) {
	if e.state.text(key) && e.state.take(1) {
		e.ObjectEncoder.AddInt64(key, v)
	}
}

// AppendInt64 delegates a bounded primitive using standard Zap encoding.
func (e *alertArrayEncoder) AppendInt64(v int64) {
	if e.state.take(1) {
		e.ArrayEncoder.AppendInt64(v)
	}
}

// AddInt32 delegates a valid, bounded primitive using standard Zap encoding.
func (e *alertObjectEncoder) AddInt32(key string, v int32) {
	if e.state.text(key) && e.state.take(1) {
		e.ObjectEncoder.AddInt32(key, v)
	}
}

// AppendInt32 delegates a bounded primitive using standard Zap encoding.
func (e *alertArrayEncoder) AppendInt32(v int32) {
	if e.state.take(1) {
		e.ArrayEncoder.AppendInt32(v)
	}
}

// AddInt16 delegates a valid, bounded primitive using standard Zap encoding.
func (e *alertObjectEncoder) AddInt16(key string, v int16) {
	if e.state.text(key) && e.state.take(1) {
		e.ObjectEncoder.AddInt16(key, v)
	}
}

// AppendInt16 delegates a bounded primitive using standard Zap encoding.
func (e *alertArrayEncoder) AppendInt16(v int16) {
	if e.state.take(1) {
		e.ArrayEncoder.AppendInt16(v)
	}
}

// AddInt8 delegates a valid, bounded primitive using standard Zap encoding.
func (e *alertObjectEncoder) AddInt8(key string, v int8) {
	if e.state.text(key) && e.state.take(1) {
		e.ObjectEncoder.AddInt8(key, v)
	}
}

// AppendInt8 delegates a bounded primitive using standard Zap encoding.
func (e *alertArrayEncoder) AppendInt8(v int8) {
	if e.state.take(1) {
		e.ArrayEncoder.AppendInt8(v)
	}
}

// AddTime delegates a valid, bounded primitive using standard Zap encoding.
func (e *alertObjectEncoder) AddTime(key string, v time.Time) {
	if e.state.text(key) && e.state.take(1) {
		e.ObjectEncoder.AddTime(key, v)
	}
}

// AppendTime delegates a bounded primitive using standard Zap encoding.
func (e *alertArrayEncoder) AppendTime(v time.Time) {
	if e.state.take(1) {
		e.ArrayEncoder.AppendTime(v)
	}
}

// AddUint delegates a valid, bounded primitive using standard Zap encoding.
func (e *alertObjectEncoder) AddUint(key string, v uint) {
	if e.state.text(key) && e.state.take(1) {
		e.ObjectEncoder.AddUint(key, v)
	}
}

// AppendUint delegates a bounded primitive using standard Zap encoding.
func (e *alertArrayEncoder) AppendUint(v uint) {
	if e.state.take(1) {
		e.ArrayEncoder.AppendUint(v)
	}
}

// AddUint64 delegates a valid, bounded primitive using standard Zap encoding.
func (e *alertObjectEncoder) AddUint64(key string, v uint64) {
	if e.state.text(key) && e.state.take(1) {
		e.ObjectEncoder.AddUint64(key, v)
	}
}

// AppendUint64 delegates a bounded primitive using standard Zap encoding.
func (e *alertArrayEncoder) AppendUint64(v uint64) {
	if e.state.take(1) {
		e.ArrayEncoder.AppendUint64(v)
	}
}

// AddUint32 delegates a valid, bounded primitive using standard Zap encoding.
func (e *alertObjectEncoder) AddUint32(key string, v uint32) {
	if e.state.text(key) && e.state.take(1) {
		e.ObjectEncoder.AddUint32(key, v)
	}
}

// AppendUint32 delegates a bounded primitive using standard Zap encoding.
func (e *alertArrayEncoder) AppendUint32(v uint32) {
	if e.state.take(1) {
		e.ArrayEncoder.AppendUint32(v)
	}
}

// AddUint16 delegates a valid, bounded primitive using standard Zap encoding.
func (e *alertObjectEncoder) AddUint16(key string, v uint16) {
	if e.state.text(key) && e.state.take(1) {
		e.ObjectEncoder.AddUint16(key, v)
	}
}

// AppendUint16 delegates a bounded primitive using standard Zap encoding.
func (e *alertArrayEncoder) AppendUint16(v uint16) {
	if e.state.take(1) {
		e.ArrayEncoder.AppendUint16(v)
	}
}

// AddUint8 delegates a valid, bounded primitive using standard Zap encoding.
func (e *alertObjectEncoder) AddUint8(key string, v uint8) {
	if e.state.text(key) && e.state.take(1) {
		e.ObjectEncoder.AddUint8(key, v)
	}
}

// AppendUint8 delegates a bounded primitive using standard Zap encoding.
func (e *alertArrayEncoder) AppendUint8(v uint8) {
	if e.state.take(1) {
		e.ArrayEncoder.AppendUint8(v)
	}
}

// AddUintptr delegates a valid, bounded primitive using standard Zap encoding.
func (e *alertObjectEncoder) AddUintptr(key string, v uintptr) {
	if e.state.text(key) && e.state.take(1) {
		e.ObjectEncoder.AddUintptr(key, v)
	}
}

// AppendUintptr delegates a bounded primitive using standard Zap encoding.
func (e *alertArrayEncoder) AppendUintptr(v uintptr) {
	if e.state.take(1) {
		e.ArrayEncoder.AppendUintptr(v)
	}
}
