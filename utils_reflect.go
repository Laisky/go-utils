package utils

import (
	"reflect"
	"runtime"
	"strconv"

	"github.com/Laisky/errors/v2"
)

// HasField check is struct has field
//
// inspired by https://mrwaggel.be/post/golang-reflect-if-initialized-struct-has-member-method-or-fields/
func HasField(st any, fieldName string) bool {
	valueIface := reflect.ValueOf(st)

	// Check if the passed interface is a pointer
	if valueIface.Type().Kind() != reflect.Pointer {
		// Create a new type of Iface's Type, so we have a pointer to work with
		valueIface = reflect.New(reflect.TypeOf(st))
	}

	// 'dereference' with Elem() and get the field by name
	field := valueIface.Elem().FieldByName(fieldName)
	return field.IsValid()
}

// HasMethod check is struct has method
//
// inspired by https://mrwaggel.be/post/golang-reflect-if-initialized-struct-has-member-method-or-fields/
func HasMethod(st any, methodName string) bool {
	valueIface := reflect.ValueOf(st)

	// Check if the passed interface is a pointer
	if valueIface.Type().Kind() != reflect.Pointer {
		// Create a new type of Iface, so we have a pointer to work with
		valueIface = reflect.New(reflect.TypeOf(st))
	}

	// Get the method by name
	method := valueIface.MethodByName(methodName)
	return method.IsValid()
}

// NilInterface make sure data is nil interface or another type with nil value
//
// Example:
//
//	type foo struct{}
//	var f *foo
//	var v any
//	v = f
//	v == nil // false
//	NilInterface(v) // true
func NilInterface(data any) bool {
	if data == nil {
		return true
	}

	if reflect.TypeOf(data).Kind() == reflect.Pointer &&
		reflect.ValueOf(data).IsNil() {
		return true
	}

	return false
}

// GetStructFieldByName get struct field by name
func GetStructFieldByName(st any, fieldName string) any {
	stv := reflect.ValueOf(st)
	if IsPtr(st) {
		stv = stv.Elem()
	}

	v := stv.FieldByName(fieldName)
	if !v.IsValid() {
		return nil
	}

	switch v.Kind() {
	case reflect.Chan,
		reflect.Func,
		reflect.Slice,
		reflect.Array,
		reflect.Interface,
		reflect.Pointer,
		reflect.Map:
		if v.IsNil() {
			return nil
		}
	default:
		// do nothing
	}

	return v.Interface()
}

// GetFuncName return the name of func
func GetFuncName(f any) string {
	return runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
}

// SetStructFieldsBySlice set field value of structs slice by values slice
func SetStructFieldsBySlice(structs, vals any) (err error) {
	sv := reflect.ValueOf(structs)
	vv := reflect.ValueOf(vals)

	typeCheck := func(name string, v *reflect.Value) error {
		switch v.Kind() {
		case reflect.Slice:
		case reflect.Array:
		default:
			return errors.New(name + " must be array/slice")
		}

		return nil
	}
	if err = typeCheck("structs", &sv); err != nil {
		return errors.WithStack(err)
	}
	if err = typeCheck("vals", &vv); err != nil {
		return errors.WithStack(err)
	}

	var (
		eachGrpValsV    reflect.Value
		iField, nFields int
	)
	for i := 0; i < Min(sv.Len(), vv.Len()); i++ {
		eachGrpValsV = vv.Index(i)
		if err = typeCheck("vals."+strconv.FormatInt(int64(i), 10), &eachGrpValsV); err != nil {
			return errors.WithStack(err)
		}
		switch sv.Index(i).Kind() {
		case reflect.Pointer:
			nFields = sv.Index(i).Elem().NumField()
		default:
			nFields = sv.Index(i).NumField()
		}
		for iField = 0; iField < Min(eachGrpValsV.Len(), nFields); iField++ {
			switch sv.Index(i).Kind() {
			case reflect.Pointer:
				sv.Index(i).Elem().Field(iField).Set(eachGrpValsV.Index(iField))
			default:
				sv.Index(i).Field(iField).Set(eachGrpValsV.Index(iField))
			}
		}
	}

	return
}

// IsPtr check if t is pointer
func IsPtr(t any) bool {
	return reflect.TypeOf(t).Kind() == reflect.Pointer
}

// IsEmpty is empty
func IsEmpty(val any) bool {
	t := reflect.TypeOf(val)
	v := reflect.ValueOf(val)
	if t.Kind() == reflect.Pointer {
		if v.IsNil() {
			return true
		}

		if v.Elem().IsZero() {
			return true
		}
	} else {
		if v.IsZero() {
			return true
		}
	}

	return false
}

// NotEmpty val should not be empty, with pretty error msg
func NotEmpty(val any, name string) error {
	t := reflect.TypeOf(val)
	v := reflect.ValueOf(val)
	if t.Kind() == reflect.Pointer {
		if v.IsNil() {
			return errors.Errorf("%q is empty pointer", name)
		}

		if v.Elem().IsZero() {
			return errors.Errorf("%q is point to empty elem", name)
		}
	} else {
		if v.IsZero() {
			return errors.Errorf("%q is empty elem", name)
		}
	}

	return nil
}

// OptionalVal return optionval if not empty
func OptionalVal[T any](ptr *T, optionalVal T) T {
	if IsEmpty(ptr) {
		return optionalVal
	}

	return *ptr
}
