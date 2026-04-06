package bot

import (
	"strings"
	"testing"
	"time"

	telebot "gopkg.in/telebot.v3"
)

// mockContext implements telebot.Context for testing.
// Only the methods actually called by the handlers are meaningfully implemented;
// the rest return zero values.
type mockContext struct {
	text    string
	sender  *telebot.User
	chat    *telebot.Chat
	message *telebot.Message
	replied string
}

func (m *mockContext) Text() string              { return m.text }
func (m *mockContext) Sender() *telebot.User     { return m.sender }
func (m *mockContext) Chat() *telebot.Chat       { return m.chat }
func (m *mockContext) Message() *telebot.Message { return m.message }
func (m *mockContext) Reply(what interface{}, _ ...interface{}) error {
	if s, ok := what.(string); ok {
		m.replied = s
	}
	return nil
}

// Unused interface methods — zero-value stubs.
func (m *mockContext) Bot() *telebot.Bot                                     { return nil }
func (m *mockContext) Update() telebot.Update                                { return telebot.Update{} }
func (m *mockContext) Callback() *telebot.Callback                           { return nil }
func (m *mockContext) Query() *telebot.Query                                 { return nil }
func (m *mockContext) InlineResult() *telebot.InlineResult                   { return nil }
func (m *mockContext) ShippingQuery() *telebot.ShippingQuery                 { return nil }
func (m *mockContext) PreCheckoutQuery() *telebot.PreCheckoutQuery           { return nil }
func (m *mockContext) Poll() *telebot.Poll                                   { return nil }
func (m *mockContext) PollAnswer() *telebot.PollAnswer                       { return nil }
func (m *mockContext) ChatMember() *telebot.ChatMemberUpdate                 { return nil }
func (m *mockContext) ChatJoinRequest() *telebot.ChatJoinRequest             { return nil }
func (m *mockContext) Migration() (int64, int64)                             { return 0, 0 }
func (m *mockContext) Topic() *telebot.Topic                                 { return nil }
func (m *mockContext) Boost() *telebot.BoostUpdated                          { return nil }
func (m *mockContext) BoostRemoved() *telebot.BoostRemoved                   { return nil }
func (m *mockContext) Recipient() telebot.Recipient                          { return nil }
func (m *mockContext) Entities() telebot.Entities                            { return nil }
func (m *mockContext) Data() string                                          { return "" }
func (m *mockContext) Args() []string                                        { return nil }
func (m *mockContext) Send(_ interface{}, _ ...interface{}) error            { return nil }
func (m *mockContext) SendAlbum(_ telebot.Album, _ ...interface{}) error     { return nil }
func (m *mockContext) Forward(_ telebot.Editable, _ ...interface{}) error    { return nil }
func (m *mockContext) ForwardTo(_ telebot.Recipient, _ ...interface{}) error { return nil }
func (m *mockContext) Edit(_ interface{}, _ ...interface{}) error            { return nil }
func (m *mockContext) EditCaption(_ string, _ ...interface{}) error          { return nil }
func (m *mockContext) EditOrSend(_ interface{}, _ ...interface{}) error      { return nil }
func (m *mockContext) EditOrReply(_ interface{}, _ ...interface{}) error     { return nil }
func (m *mockContext) Delete() error                                         { return nil }
func (m *mockContext) DeleteAfter(_ time.Duration) *time.Timer               { return nil }
func (m *mockContext) Notify(_ telebot.ChatAction) error                     { return nil }
func (m *mockContext) Ship(_ ...interface{}) error                           { return nil }
func (m *mockContext) Accept(_ ...string) error                              { return nil }
func (m *mockContext) Answer(_ *telebot.QueryResponse) error                 { return nil }
func (m *mockContext) Respond(_ ...*telebot.CallbackResponse) error          { return nil }
func (m *mockContext) RespondText(_ string) error                            { return nil }
func (m *mockContext) RespondAlert(_ string) error                           { return nil }
func (m *mockContext) Get(_ string) interface{}                              { return nil }
func (m *mockContext) Set(_ string, _ interface{})                           {}

// --- OnText ---

func TestOnText_StartCommand(t *testing.T) {
	h := newTestHandler(newTestQueue(10, &mockProcessor{}))

	ctx := &mockContext{
		text:   "/start",
		sender: &telebot.User{ID: 1, FirstName: "Alice"},
	}

	if err := h.OnText(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(ctx.replied, "Alice") {
		t.Errorf("reply should contain sender name, got: %q", ctx.replied)
	}
	if !strings.Contains(ctx.replied, "/list") {
		t.Errorf("reply should list available commands, got: %q", ctx.replied)
	}
}

func TestOnText_UnknownCommand(t *testing.T) {
	h := newTestHandler(newTestQueue(10, &mockProcessor{}))

	ctx := &mockContext{
		text:   "/unknown",
		sender: &telebot.User{ID: 2},
	}

	if err := h.OnText(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(ctx.replied, "Unknown command") {
		t.Errorf("reply should contain 'Unknown command', got: %q", ctx.replied)
	}
}

// --- OnVoice ---

func TestOnVoice_AcknowledgesAndSubmitsJob(t *testing.T) {
	q := newTestQueue(10, &mockProcessor{})
	h := newTestHandler(q)

	ctx := &mockContext{
		sender: &telebot.User{ID: 42},
		chat:   &telebot.Chat{ID: 100},
		message: &telebot.Message{
			Voice: &telebot.Voice{
				File:     telebot.File{FileID: "voice-abc"},
				Duration: 10,
				MIME:     "audio/ogg",
			},
		},
	}

	if err := h.OnVoice(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.replied == "" {
		t.Fatal("expected an acknowledgement reply, got none")
	}
	if len(q.jobs) != 1 {
		t.Fatalf("expected 1 job in queue, got %d", len(q.jobs))
	}

	job := <-q.jobs
	if job.FileID != "voice-abc" {
		t.Errorf("FileID: want %q, got %q", "voice-abc", job.FileID)
	}
	if job.UserID != 42 {
		t.Errorf("UserID: want 42, got %d", job.UserID)
	}
	if job.ChatID != 100 {
		t.Errorf("ChatID: want 100, got %d", job.ChatID)
	}
	if job.Type != JobTypeTranscribe {
		t.Errorf("Type: want %q, got %q", JobTypeTranscribe, job.Type)
	}
}

// --- OnAudio ---

func TestOnAudio_AcknowledgesAndSubmitsJob(t *testing.T) {
	q := newTestQueue(10, &mockProcessor{})
	h := newTestHandler(q)

	ctx := &mockContext{
		sender: &telebot.User{ID: 7},
		chat:   &telebot.Chat{ID: 77},
		message: &telebot.Message{
			Audio: &telebot.Audio{
				File:     telebot.File{FileID: "audio-xyz"},
				FileName: "meeting.mp3",
				MIME:     "audio/mpeg",
			},
		},
	}

	if err := h.OnAudio(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.replied == "" {
		t.Fatal("expected an acknowledgement reply, got none")
	}
	if len(q.jobs) != 1 {
		t.Fatalf("expected 1 job in queue, got %d", len(q.jobs))
	}

	job := <-q.jobs
	if job.FileID != "audio-xyz" {
		t.Errorf("FileID: want %q, got %q", "audio-xyz", job.FileID)
	}
	if job.FileName != "meeting.mp3" {
		t.Errorf("FileName: want %q, got %q", "meeting.mp3", job.FileName)
	}
}
