package algorithm

import (
	"slices"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/go-utils/v4/common"
)

// GetLargestNItems get N highest priority items
func GetLargestNItems[T common.Sortable](inputChan <-chan T, topN int) ([]T, error) {
	result, err := GetTopKItems(inputChan, topN, common.SortOrderDesc)
	if err != nil {
		return nil, errors.Wrap(err, "get topK items")
	}

	// sort by min-heap, so reverse it
	slices.Reverse(result)
	return result, nil
}

// GetSmallestNItems get N smallest priority items
func GetSmallestNItems[T common.Sortable](inputChan <-chan T, topN int) ([]T, error) {
	result, err := GetTopKItems(inputChan, topN, common.SortOrderAsc)
	if err != nil {
		return nil, errors.Wrap(err, "get topK items")
	}

	// sort by max-heap, so reverse it
	slices.Reverse(result)
	return result, nil
}

// GetTopKItems calculate topN by heap
func GetTopKItems[T common.Sortable](
	inputChan <-chan T,
	topN int,
	sortOrder common.SortOrder,
) (result []T, err error) {
	if topN < 1 {
		return nil, errors.Errorf("topN must greater than 0")
	}

	var heapSort common.SortOrder
	switch sortOrder {
	case common.SortOrderAsc:
		heapSort = common.SortOrderDesc
	case common.SortOrderDesc:
		heapSort = common.SortOrderAsc
	default:
		return nil, errors.Errorf("unsupported sort order %v", sortOrder)
	}

	q := NewPriorityQ[T](heapSort)
	for v := range inputChan {
		q.Push(PriorityItem[T]{
			Val: v,
		})
		if q.Len() > topN {
			q.Pop()
		}
	}

	result = make([]T, 0, topN)
	for q.Len() != 0 {
		it := q.Pop()
		result = append(result, it.GetVal())
	}

	return result, nil
}
