package gigachat

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTruncateRunes_NoTruncation(t *testing.T) {
	s := "привет мир"
	if got := truncateRunes(s, 100); got != s {
		t.Errorf("got %q, want %q", got, s)
	}
}

func TestTruncateRunes_TruncatesAndSuffix(t *testing.T) {
	s := strings.Repeat("а", 50)
	wantPrefix := strings.Repeat("а", 10)
	got := truncateRunes(s, 10)
	if !strings.HasSuffix(got, "…(текст обрезан)") {
		t.Errorf("expected ellipsis suffix, got %q", got)
	}
	if !strings.HasPrefix(got, wantPrefix) {
		t.Errorf("expected prefix %d×'а', got %q", 10, got)
	}
}

func TestChatResponse_JSONDecode(t *testing.T) {
	raw := `{"choices":[{"message":{"role":"assistant","content":"  Ответ  \n"}}]}`
	var out chatResponse
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Choices) != 1 || out.Choices[0].Message.Content != "  Ответ  \n" {
		t.Fatalf("unexpected decode: %+v", out)
	}
}

func TestStubClient_AnswerNotConfigured(t *testing.T) {
	var s stubClient
	_, err := s.Answer(t.Context(), "ctx", "q")
	if err != ErrNotConfigured {
		t.Fatalf("want ErrNotConfigured, got %v", err)
	}
}

func TestStubClient_SummarizeEmpty(t *testing.T) {
	var s stubClient
	sum, err := s.SummarizeTranscript(t.Context(), "  ")
	if err != nil || sum != "" {
		t.Fatalf("want empty summary, err=nil; got %q, %v", sum, err)
	}
}
