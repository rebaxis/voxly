package bot

import (
	"context"
	"sync"

	"github.com/voxly/voxly/internal/config"
	"github.com/voxly/voxly/internal/lib/logger"
	"go.uber.org/zap"
)

// JobType identifies the kind of work to be done.
type JobType string

const JobTypeTranscribe JobType = "transcribe"

// Job represents a unit of work submitted to the queue.
type Job struct {
	UserID   int64
	ChatID   int64
	Type     JobType
	FileID   string
	FileName string
	MimeType string
}

// JobResult carries the outcome of a processed Job back to the dispatcher.
type JobResult struct {
	UserID int64
	ChatID int64
	Text   string
	Err    error
}

// Queue is a buffered work queue backed by goroutine workers.
// Jobs are submitted via Submit and results are read from Results.
type Queue struct {
	jobs      chan Job
	results   chan JobResult
	processor Processor
	log       *logger.Logger
	wg        sync.WaitGroup
}

// NewQueue constructs a Queue with channel sizes taken from cfg.
func NewQueue(cfg *config.Config, processor Processor, log *logger.Logger) *Queue {
	return &Queue{
		jobs:      make(chan Job, cfg.QueueSize),
		results:   make(chan JobResult, cfg.QueueSize),
		processor: processor,
		log:       log.WithComponent("queue"),
	}
}

// Submit enqueues a job for processing. If the queue is full the job is dropped
// and a warning is logged rather than blocking the caller.
func (q *Queue) Submit(job Job) {
	select {
	case q.jobs <- job:
		q.log.Info("job submitted",
			zap.String("type", string(job.Type)),
			zap.Int64("user_id", job.UserID),
		)
	default:
		q.log.Warn("queue is full, dropping job",
			zap.String("type", string(job.Type)),
			zap.Int64("user_id", job.UserID),
		)
	}
}

// Results returns the read-only channel of job results.
func (q *Queue) Results() <-chan JobResult {
	return q.results
}

// StartWorkers launches count worker goroutines. Each worker exits when the
// jobs channel is closed (via Stop) or when ctx is cancelled.
func (q *Queue) StartWorkers(ctx context.Context, count int) {
	for i := range count {
		q.wg.Add(1)
		go q.worker(ctx, i)
	}
	q.log.Info("workers started", zap.Int("count", count))
}

// Stop closes the jobs channel, waits for all workers to drain and finish,
// then closes the results channel.
func (q *Queue) Stop() {
	close(q.jobs)
	q.wg.Wait()
	close(q.results)
	q.log.Info("queue stopped")
}

func (q *Queue) worker(ctx context.Context, id int) {
	defer q.wg.Done()
	q.log.Info("worker started", zap.Int("id", id))

	for job := range q.jobs {
		select {
		case <-ctx.Done():
			q.log.Info("worker context cancelled", zap.Int("id", id))
			return
		default:
			q.processJob(ctx, job)
		}
	}

	q.log.Info("worker stopped", zap.Int("id", id))
}

func (q *Queue) processJob(ctx context.Context, job Job) {
	q.log.Info("processing job",
		zap.String("type", string(job.Type)),
		zap.Int64("user_id", job.UserID),
		zap.String("file_id", job.FileID),
	)

	text, err := q.processor.Process(ctx, job)

	q.results <- JobResult{
		UserID: job.UserID,
		ChatID: job.ChatID,
		Text:   text,
		Err:    err,
	}
}
