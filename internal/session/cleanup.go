package session

import (
	"context"
	"log/slog"
	"time"
)

func StartCleanupLoop(ctx context.Context, table *Table, interval time.Duration, logger *slog.Logger) <-chan int {
	out := make(chan int, 1)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer close(out)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				deleted := table.CleanupExpired(time.Now().UTC())
				if len(deleted) > 0 {
					logger.Info("expired sessions cleaned", "count", len(deleted))
					select {
					case out <- len(deleted):
					default:
					}
				}
			}
		}
	}()
	return out
}
