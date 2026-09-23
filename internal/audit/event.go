package audit

import "time"

// Event describes an audit event.
type Event struct {
	Time    time.Time
	Message string
}
