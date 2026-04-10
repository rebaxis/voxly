package gigachat

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	gocache "github.com/patrickmn/go-cache"
	"github.com/voxly/voxly/internal/lib/logger"
	"go.uber.org/zap"
)

// ErrNotConfigured is returned by the stub when GigaChat credentials are absent.
var ErrNotConfigured = errors.New("gigachat not configured")

const (
	oauthEndpoint = "https://ngw.devices.sberbank.ru:9443/api/v2/oauth"
	// Chat completions for individuals (физлиц, e.g. GIGACHAT_API_PERS). See:
	// https://developers.sber.ru/docs/ru/gigachat/api/reference/rest/post-chat
	// Legal entities use base https://api.giga.chat/v1 instead (same /chat/completions path).
	chatEndpoint = "https://gigachat.devices.sberbank.ru/api/v1/chat/completions"

	tokenCacheKey   = "gigachat_access_token"
	tokenBufferSecs = 60

	maxContextRunes = 12000
)

// newTransport returns an HTTP transport tuned for Sber GigaChat: TLS with verification
// skipped (custom CA), HTTP/1.1 only (default HTTP/2 often yields EOF on chat/completions),
// explicit dial timeout, and no TLS handshake deadline (ngw can be slow).
func newTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   60 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2: false,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // Sber uses custom CA
			MinVersion:         tls.VersionTLS12,
			NextProtos:         []string{"http/1.1"},
		},
		// Empty map disables net/http's HTTP/2 upgrade for this transport.
		TLSNextProto:    make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
		MaxIdleConns:    100,
		IdleConnTimeout: 90 * time.Second,
		// Zero = no limit. A short timeout breaks OAuth to ngw:9443 on slower networks.
		TLSHandshakeTimeout:   60 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

// Client calls GigaChat REST API (OAuth + chat completions).
type Client interface {
	SummarizeTranscript(ctx context.Context, transcript string) (summary string, err error)
	Answer(ctx context.Context, contextText, question string) (answer string, err error)
}

// Config holds GigaChat OAuth credentials (Authorization Key from Sber Studio, same style as SaluteSpeech).
type Config struct {
	AuthorizationKey string
	Scope            string
	Model            string
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
			Role    string `json:"role"`
		} `json:"message"`
	} `json:"choices"`
}

type httpClient struct {
	cfg        Config
	http       *http.Client
	tokenCache *gocache.Cache
	log        *logger.Logger
}

// New returns a Client that talks to GigaChat over HTTPS.
func New(cfg Config, log *logger.Logger) Client {
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "GigaChat"
	}
	return &httpClient{
		cfg: Config{
			AuthorizationKey: cfg.AuthorizationKey,
			Scope:            cfg.Scope,
			Model:            model,
		},
		http: &http.Client{
			Transport: newTransport(),
			Timeout:   0,
		},
		tokenCache: gocache.New(25*time.Minute, 5*time.Minute),
		log:        log.WithComponent("gigachat"),
	}
}

// NewStub returns a Client that skips summarization and refuses Answer (ErrNotConfigured).
func NewStub() Client { return stubClient{} }

type stubClient struct{}

func (stubClient) SummarizeTranscript(context.Context, string) (string, error) {
	return "", nil
}

func (stubClient) Answer(context.Context, string, string) (string, error) {
	return "", ErrNotConfigured
}

func (c *httpClient) SummarizeTranscript(ctx context.Context, transcript string) (string, error) {
	transcript = strings.TrimSpace(transcript)
	if transcript == "" {
		return "", nil
	}
	body := truncateRunes(transcript, maxContextRunes)
	return c.chat(ctx, []chatMessage{
		{Role: "system", Content: "Ты помогаешь с деловыми встречами. Сделай краткое резюме следующей расшифровки на русском языке (3–6 предложений). Не выдумывай факты, которых нет в тексте."},
		{Role: "user", Content: body},
	})
}

func (c *httpClient) Answer(ctx context.Context, contextText, question string) (string, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return "", fmt.Errorf("empty question")
	}
	var b strings.Builder
	if ctxText := strings.TrimSpace(contextText); ctxText != "" {
		b.WriteString("Контекст (расшифровка встречи):\n")
		b.WriteString(truncateRunes(ctxText, maxContextRunes))
		b.WriteString("\n\n")
	}
	b.WriteString("Вопрос пользователя:\n")
	b.WriteString(question)
	return c.chat(ctx, []chatMessage{
		{Role: "system", Content: "Ты — ассистент Voxly. Отвечай по-русски точно и по делу. Если есть контекст встречи, опирайся только на него; иначе отвечай как общий ассистент."},
		{Role: "user", Content: b.String()},
	})
}

func truncateRunes(s string, max int) string {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "\n…(текст обрезан)"
}

func (c *httpClient) authorizationHeader() (string, error) {
	raw := strings.TrimSpace(c.cfg.AuthorizationKey)
	raw = strings.ReplaceAll(raw, "\n", "")
	raw = strings.ReplaceAll(raw, "\r", "")
	if raw == "" {
		return "", fmt.Errorf("set gigachat_authorization_key")
	}
	if strings.HasPrefix(strings.ToLower(raw), "basic ") {
		return raw, nil
	}
	return "Basic " + raw, nil
}

func (c *httpClient) accessToken(ctx context.Context) (string, error) {
	if tok, ok := c.tokenCache.Get(tokenCacheKey); ok {
		return tok.(string), nil
	}

	authHdr, err := c.authorizationHeader()
	if err != nil {
		return "", err
	}

	scope := strings.TrimSpace(c.cfg.Scope)
	if scope == "" {
		scope = "GIGACHAT_API_PERS"
	}
	form := url.Values{}
	form.Set("scope", scope)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauthEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("gigachat oauth new request: %w", err)
	}
	req.Header.Set("Authorization", authHdr)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("RqUID", uuid.New().String())

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("gigachat oauth request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, rerr := io.ReadAll(resp.Body)
		if rerr != nil {
			return "", fmt.Errorf("gigachat oauth status %d: read body: %w", resp.StatusCode, rerr)
		}
		return "", fmt.Errorf("gigachat oauth status %d: %s", resp.StatusCode, body)
	}

	var tok tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", fmt.Errorf("gigachat decode token: %w", err)
	}

	ttl := time.Duration(tok.ExpiresIn-tokenBufferSecs) * time.Second
	if ttl < time.Minute {
		ttl = time.Minute
	}
	c.tokenCache.Set(tokenCacheKey, tok.AccessToken, ttl)
	c.log.Info("GigaChat token refreshed", zap.Duration("ttl", ttl))
	return tok.AccessToken, nil
}

func (c *httpClient) chat(ctx context.Context, messages []chatMessage) (string, error) {
	token, err := c.accessToken(ctx)
	if err != nil {
		return "", err
	}

	payload := chatRequest{
		Model:    c.cfg.Model,
		Messages: messages,
		Stream:   false,
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("gigachat marshal chat: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, chatEndpoint, bytes.NewReader(buf))
	if err != nil {
		return "", fmt.Errorf("gigachat chat new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Request-ID", uuid.New().String())

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("gigachat chat request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("gigachat chat read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gigachat chat status %d: %s", resp.StatusCode, body)
	}

	var out chatResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("gigachat chat decode: %w", err)
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("gigachat empty completion")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}
