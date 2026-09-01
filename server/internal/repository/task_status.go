package repository

import "time"

// nextCompletedAt is the completed_at value after a status write.
// Entering done keeps an existing timestamp (first completion wins) or stamps
// now. Leaving done clears it so a later re-done records a new time.
func nextCompletedAt(newStatus string, current *time.Time, now time.Time) *time.Time {
	if newStatus != "done" {
		return nil
	}
	if current != nil {
		return current
	}
	t := now
	return &t
}

func shouldRecordStatusHistory(oldStatus, newStatus string) bool {
	return oldStatus != newStatus
}
