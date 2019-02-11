package journal

//go:generate msgp

type Data struct {
	Data map[string]interface{}
	ID   int64
}
