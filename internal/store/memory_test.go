package store

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/coolspartak9819-rgb/flashsale-reservation-platform/internal/domain"
)

func TestReserveAllowsOnlyOneConcurrentWinner(t *testing.T) {
	s := New()
	if err := s.CreateEvent(domain.Event{ID: "concert", Name: "Concert", SeatCount: 1}); err != nil {
		t.Fatal(err)
	}
	const attempts = 100
	results := make(chan error, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := s.Reserve("concert", fmt.Sprintf("user-%d", i), "seat-1", fmt.Sprintf("key-%d", i), 10*time.Minute)
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	twins := 0
	for err := range results {
		if err == nil {
			twins++
		}
		if err != nil && !errors.Is(err, ErrSeatTaken) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if twins != 1 {
		t.Fatalf("expected exactly one winner, got %d", twins)
	}
}
