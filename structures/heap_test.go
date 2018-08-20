package structures_test

import (
	"fmt"
	"testing"

	utils "github.com/Laisky/go-utils"
	"github.com/Laisky/go-utils/structures"
)

var (
	itemsWaitToSort = []*structures.Item{
		&structures.Item{Priority: 1},
		&structures.Item{Priority: 3},
		&structures.Item{Priority: 55},
		&structures.Item{Priority: 2},
		&structures.Item{Priority: 4441},
		&structures.Item{Priority: 15555},
		&structures.Item{Priority: 122},
	}
)

func ExampleGetLargestNItems() {
	var (
		itemsWaitToSort = []*structures.Item{
			&structures.Item{Priority: 1},
			&structures.Item{Priority: 3},
			&structures.Item{Priority: 55},
			&structures.Item{Priority: 2},
			&structures.Item{Priority: 4441},
			&structures.Item{Priority: 15555},
			&structures.Item{Priority: 122},
		}
		itemChan = make(chan structures.ItemItf)
	)

	go func() {
		for _, item := range itemsWaitToSort {
			itemChan <- item
		}

		close(itemChan)
	}()

	items, err := structures.GetLargestNItems(itemChan, 3)
	if err != nil {
		panic(err)
	}

	for _, item := range items {
		// 15555
		// 4441
		// 112
		fmt.Println(item.GetPriority())
	}
}

func ExampleGetSmallestNItems() {
	var (
		itemsWaitToSort = []*structures.Item{
			&structures.Item{Priority: 1},
			&structures.Item{Priority: 3},
			&structures.Item{Priority: 55},
			&structures.Item{Priority: 2},
			&structures.Item{Priority: 4441},
			&structures.Item{Priority: 15555},
			&structures.Item{Priority: 122},
		}
		itemChan = make(chan structures.ItemItf)
	)

	go func() {
		for _, item := range itemsWaitToSort {
			itemChan <- item
		}

		close(itemChan)
	}()

	items, err := structures.GetSmallestNItems(itemChan, 3)
	if err != nil {
		panic(err)
	}

	for _, item := range items {
		// 1
		// 2
		// 3
		fmt.Println(item.GetPriority())
	}
}

func TestGetTopKItems(t *testing.T) {
	// defer utils.Logger.Sync()
	generate := func(itemChan chan structures.ItemItf) {
		for _, item := range itemsWaitToSort {
			itemChan <- item
		}

		close(itemChan)
	}

	var (
		items    []structures.ItemItf
		err      error
		itemChan chan structures.ItemItf
	)

	// test highest
	itemChan = make(chan structures.ItemItf)
	go generate(itemChan)
	items, err = structures.GetTopKItems(itemChan, 3, true)
	if err != nil {
		t.Errorf("%+v", err)
	}

	if items[0].GetPriority() != 15555 {
		t.Errorf("expect 15555, got %+v", items[0].GetPriority())
	}
	if items[1].GetPriority() != 4441 {
		t.Errorf("expect 4441, got %+v", items[1].GetPriority())
	}
	if items[2].GetPriority() != 122 {
		t.Errorf("expect 122, got %+v", items[2].GetPriority())
	}

	// test lowest
	itemChan = make(chan structures.ItemItf)
	go generate(itemChan)
	items, err = structures.GetTopKItems(itemChan, 3, false)
	if err != nil {
		t.Errorf("%+v", err)
	}

	if items[0].GetPriority() != 1 {
		t.Errorf("expect 1, got %+v", items[0].GetPriority())
	}
	if items[1].GetPriority() != 2 {
		t.Errorf("expect 2, got %+v", items[1].GetPriority())
	}
	if items[2].GetPriority() != 3 {
		t.Errorf("expect 3, got %+v", items[2].GetPriority())
	}
}

func init() {
	utils.SetupLogger("debug")
}
