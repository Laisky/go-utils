package journal

//go:generate msgp

// Data msgp data schema
type Data struct {
	Data map[string]interface{}
	ID   int64
}
