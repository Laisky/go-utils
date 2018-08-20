package structures

import (
	"container/heap"
	"fmt"

	utils "github.com/Laisky/go-utils"
	"go.uber.org/zap"
)

type Item struct {
	Priority int
	Key      interface{}
	idx      int
}

func (it *Item) GetKey() interface{} {
	return it.Key
}

func (it *Item) GetPriority() int {
	return it.Priority
}

type PriorityQ struct {
	isHighPriority bool
	topN           int
	q              []*Item
}

func NewPriorityQ(isHighPriority bool, topN int) *PriorityQ {
	utils.Logger.Debug("create PriorityQ", zap.Int("topN", topN))
	return &PriorityQ{
		isHighPriority: isHighPriority,
		topN:           topN,
		q:              []*Item{},
	}
}

func (p *PriorityQ) Len() int {
	utils.Logger.Debug("len", zap.Int("len", len(p.q)))
	return len(p.q)
}

func (p *PriorityQ) Less(i, j int) bool {
	utils.Logger.Debug("less two items", zap.Int("i", i), zap.Int("j", j))
	if p.isHighPriority {
		return p.q[i].Priority < p.q[j].Priority
	}

	return p.q[i].Priority > p.q[j].Priority
}

func (p *PriorityQ) Swap(i, j int) {
	utils.Logger.Debug("swap two items", zap.Int("i", i), zap.Int("j", j))
	p.q[i], p.q[j] = p.q[j], p.q[i]
	p.q[i].idx = i
	p.q[j].idx = j
}

func (p *PriorityQ) Push(x interface{}) {
	utils.Logger.Debug("push item", zap.Int("priority", x.(*Item).GetPriority()))
	n := p.Len()
	item := x.(*Item)
	item.idx = n
	p.q = append(p.q, item)
}

func (p *PriorityQ) Pop() (popped interface{}) {
	utils.Logger.Debug("pop item")
	n := p.Len()
	item := p.q[n-1]
	item.idx = -1
	p.q = p.q[:n-1]
	return item
}

// ItemItf items need to sort
type ItemItf interface {
	GetKey() interface{}
	GetPriority() int
}

// GetLargestNItems get N highest priority items
func GetLargestNItems(inputChan <-chan ItemItf, topN int) ([]ItemItf, error) {
	return GetTopKItems(inputChan, topN, true)
}

// GetSmallestNItems get N smallest priority items
func GetSmallestNItems(inputChan <-chan ItemItf, topN int) ([]ItemItf, error) {
	return GetTopKItems(inputChan, topN, false)
}

// GetTopKItems calculate topN by heap
// use min-heap to calculates topN Highest items
// use max-heap to calculates topN Lowest items
func GetTopKItems(inputChan <-chan ItemItf, topN int, isHighest bool) ([]ItemItf, error) {
	utils.Logger.Debug("GetMostFreqWords for key2PriMap", zap.Int("topN", topN))
	if topN < 2 {
		return nil, fmt.Errorf("GetMostFreqWords topN must larger than 2")
	}

	var (
		i               int
		ok              bool
		item, thresItem ItemItf
		items           = make([]ItemItf, topN)
		nTotal          = 0
		p               = NewPriorityQ(isHighest, topN)
	)

	for i = 0; i < topN; i++ { // load first topN items
		select {
		case item, ok = <-inputChan:
			if !ok { // channel closed
				inputChan = nil
				break
			}
			nTotal++
			if nTotal == 1 {
				thresItem = item
			}

			if (isHighest && item.GetPriority() < thresItem.GetPriority()) ||
				(!isHighest && item.GetPriority() > thresItem.GetPriority()) {
				thresItem = item
			}

			p.Push(&Item{
				Key:      item.GetKey(),
				Priority: item.GetPriority(),
			})
		}
	}

	if inputChan == nil {
		if p.Len() == 1 { // only one item
			return []ItemItf{item}, nil
		}
		if p.Len() == 0 {
			return []ItemItf{}, nil
		}
	}

	heap.Init(p) // initialize heap

	// load all remain items
	if inputChan != nil {
		for item = range inputChan {
			nTotal++
			if (isHighest && item.GetPriority() <= thresItem.GetPriority()) ||
				(!isHighest && item.GetPriority() >= thresItem.GetPriority()) {
				continue
			}

			heap.Push(p, &Item{
				Priority: item.GetPriority(),
				Key:      item.GetKey(),
			})
			thresItem = heap.Pop(p).(*Item)
		}
	}

	utils.Logger.Debug("process all items", zap.Int("total", nTotal))
	for i := 1; i <= topN; i++ { // pop all needed items
		item = heap.Pop(p).(*Item)
		items[topN-i] = item
	}

	return items, nil
}
