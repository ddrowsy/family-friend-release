package audit

import "log"

// Logger records audit events.
type Logger struct {
	logger *log.Logger
}

// NewLogger creates an audit logger.
func NewLogger(logger *log.Logger) Logger {
	if logger == nil {
		logger = log.Default()
	}

	return Logger{logger: logger}
}

// Log records an event.
func (l Logger) Log(event Event) error {
	logger := l.logger
	if logger == nil {
		logger = log.Default()
	}

	logger.Printf("audit event written: %s", event.Message)
	return nil
}
