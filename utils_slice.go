package utils

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/go-utils/v6/algorithm"
	"github.com/Laisky/go-utils/v6/common"
)

// UniqueStrings remove duplicate string in slice
func UniqueStrings(vs []string) []string {
	seen := make(map[string]struct{})
	j := 0
	for _, v := range vs {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			vs[j] = v
			j++
		}
	}

	clear(vs[j:])
	return vs[:j:j]
}

// RemoveEmpty remove duplicate string in slice
func RemoveEmpty(vs []string) (r []string) {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			r = append(r, v)
		}
	}

	return
}

// TrimEleSpaceAndRemoveEmpty remove duplicate string in slice
func TrimEleSpaceAndRemoveEmpty(vs []string) (r []string) {
	for _, v := range vs {
		v = strings.TrimSpace(v)
		if v != "" {
			r = append(r, v)
		}
	}

	return
}

// Contains if collection contains ele
func Contains[V comparable](collection []V, ele V) bool {
	return slices.Contains(collection, ele)
}

// ConvertMap2StringKey convert any map to `map[string]any`
func ConvertMap2StringKey(inputMap any) map[string]any {
	v := reflect.ValueOf(inputMap)
	if v.Kind() != reflect.Map {
		return nil
	}

	m2 := map[string]any{}
	ks := v.MapKeys()
	for _, k := range ks {
		if k.Kind() == reflect.Interface {
			m2[k.Elem().String()] = v.MapIndex(k).Interface()
		} else {
			m2[fmt.Sprint(k)] = v.MapIndex(k).Interface()
		}
	}

	return m2
}

// ReverseSlice reverse slice
func ReverseSlice[T any](s []T) {
	for i := len(s)/2 - 1; i >= 0; i-- {
		opp := len(s) - 1 - i
		s[i], s[opp] = s[opp], s[i]
	}
}

// RemoveEmptyVal remove empty value in map
func RemoveEmptyVal(m map[string]any) map[string]any {
	for k, v := range m {
		if v == nil || reflect.ValueOf(v).IsZero() {
			delete(m, k)
			continue
		}

		switch reflect.TypeOf(v).Kind() {
		case reflect.Map:
			if v == nil || reflect.ValueOf(v).Len() == 0 {
				delete(m, k)
				continue
			}

			switch v := v.(type) {
			case map[string]any:
				v = RemoveEmptyVal(v)
				if len(v) == 0 {
					delete(m, k)
					continue
				}
				m[k] = v
			default:
				continue
			}
		case reflect.Slice, reflect.Array:
			if v == nil || reflect.ValueOf(v).Len() == 0 {
				delete(m, k)
				continue
			}
		default:
			continue
		}
	}

	return m
}

// CombineSortedChain return the intersection of multiple sorted chans
func CombineSortedChain[T Sortable](sortOrder common.SortOrder, chans ...chan T) (result chan T, err error) {
	if len(chans) == 0 {
		return nil, errors.New("chans cannot be empty")
	} else if len(chans) == 1 {
		return chans[0], nil
	}

	heap := algorithm.NewPriorityQ[T](sortOrder)

	activeChans := make(map[int]chan T, len(chans))
	for i, ch := range chans {
		activeChans[i] = ch
	}

	result = make(chan T)
	go func() {
		defer close(result)

		for idx, c := range activeChans {
			v, ok := <-c
			if !ok {
				continue
			}

			heap.Push(algorithm.PriorityItem[T]{
				Val:  v,
				Name: idx,
			})
		}

		for {
			if heap.Len() == 0 {
				return
			}

			it := heap.Pop()
			result <- it.GetVal()

			idx := it.(algorithm.PriorityItem[T]).Name.(int) //nolint:forcetypeassert // panic
			ch, ok := activeChans[idx]
			if !ok { // this chan is already exhausted and removed
				continue
			}

			v, ok := <-ch
			if !ok { // this chan is exhausted
				delete(activeChans, idx)

				// there is no active chans
				if len(activeChans) == 0 {
					for i := 0; i < heap.Len(); i++ {
						it := heap.Pop()
						result <- it.GetVal()
					}

					return
				}

				continue
			}

			heap.Push(algorithm.PriorityItem[T]{
				Val:  v,
				Name: idx,
			})
		}
	}()

	return result, nil
}

// FilterSlice filters a slice inplace
func FilterSlice[T any](s []T, f func(v T) bool) []T {
	var j int
	for _, v := range s {
		if f(v) {
			s[j] = v
			j++
		}
	}

	clear(s[j:])
	return s[:j:j]
}
