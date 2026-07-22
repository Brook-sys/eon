package main

import (
	"context"
	"fmt"
	"sync"
	"time"
	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/sqlite"
)

func main() {
	store, err := sqlite.Open(":memory:")
	if err != nil { panic(err) }
	
	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ctx := context.Background()
			store.Update(ctx, func(tx port.Transaction) error {
				event := domain.Event{
					SchemaVersion: domain.SchemaVersionV1,
					ID: domain.EventID(fmt.Sprintf("evt-%d", id)),
					Kind: "test.kind",
					OccurredAt: time.Now().UTC(),
				}
				_, err := tx.AppendEvent(event)
				return err
			})
		}(i)
	}
	wg.Wait()
	
	_ = store.View(context.Background(), func(reader port.Reader) error {
		events, _ := reader.Events(0, 2000)
		fmt.Printf("Total events: %d\n", len(events))
		
		for i := 0; i < len(events); i++ {
			if events[i].Sequence != uint64(i+1) {
				fmt.Printf("Gap or disorder at index %d: seq=%d\n", i, events[i].Sequence)
				break
			}
		}
		return nil
	})
}
