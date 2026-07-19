package domain

type MemoryStoredEvent struct {
	ID       string
	MemoryID MemoryID
	Key      string
	Scope    MemoryScope
	At       string
}

type MemoryCompactedEvent struct {
	ID       string
	MemoryID MemoryID
	Reason   string
	At       string
}
