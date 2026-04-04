package bot

import (
	"context"

	"github.com/voxly/voxly/internal/lib/logger"
	"go.uber.org/zap"
)

// newTestLogger returns a no-op logger suitable for tests.
func newTestLogger() *logger.Logger {
	return &logger.Logger{Logger: zap.NewNop()}
}

// mockProcessor is a Processor that returns a fixed result without doing any I/O.
type mockProcessor struct {
	result string
	err    error
}

func (m *mockProcessor) Process(_ context.Context, _ Job) (string, error) {
	return m.result, m.err
}

// newTestQueue creates a Queue with a buffered channel of the given size and a
// mockProcessor. No workers are started — callers control that explicitly.
func newTestQueue(size int, proc Processor) *Queue {
	return &Queue{
		jobs:      make(chan Job, size),
		results:   make(chan JobResult, size),
		processor: proc,
		log:       newTestLogger().WithComponent("queue"),
	}
}
