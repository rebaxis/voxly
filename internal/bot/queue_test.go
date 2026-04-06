package bot

import (
	"errors"
	"testing"
)

func TestQueue_SubmitJobIsProcessed(t *testing.T) {
	proc := &mockProcessor{result: "transcribed text"}
	q := newTestQueue(10, proc)

	job := Job{UserID: 1, ChatID: 10, Type: JobTypeTranscribe, FileID: "file1"}

	q.StartWorkers(1, nil)

	q.Submit(job)

	q.Stop()

	result, ok := <-q.Results()
	if !ok {
		t.Fatal("results channel closed before receiving result")
	}
	if result.Err != nil {
		t.Fatalf("unexpected error in result: %v", result.Err)
	}
	if result.Text != "transcribed text" {
		t.Errorf("Text: want %q, got %q", "transcribed text", result.Text)
	}
	if result.ChatID != 10 {
		t.Errorf("ChatID: want 10, got %d", result.ChatID)
	}
}

func TestQueue_ProcessorErrorPropagatedToResult(t *testing.T) {
	proc := &mockProcessor{err: errors.New("download failed")}
	q := newTestQueue(10, proc)

	q.StartWorkers(1, nil)
	q.Submit(Job{UserID: 2, ChatID: 20, Type: JobTypeTranscribe, FileID: "file2"})
	q.Stop()

	result := <-q.Results()
	if result.Err == nil {
		t.Fatal("expected an error in result, got nil")
	}
	if result.Err.Error() != "download failed" {
		t.Errorf("Err: want %q, got %q", "download failed", result.Err.Error())
	}
}

func TestQueue_FullQueueDropsJobWithoutBlocking(t *testing.T) {
	proc := &mockProcessor{result: "ok"}
	q := newTestQueue(1, proc) // buffer of exactly 1

	q.Submit(Job{FileID: "first"})  // fills the buffer
	q.Submit(Job{FileID: "second"}) // must be dropped, not block

	if len(q.jobs) != 1 {
		t.Errorf("expected 1 job in buffer, got %d", len(q.jobs))
	}
}
