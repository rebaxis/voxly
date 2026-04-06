package salutespeech

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	gocache "github.com/patrickmn/go-cache"
	"github.com/voxly/voxly/internal/lib/logger"
	"go.uber.org/zap"
)

const (
	authEndpoint   = "https://ngw.devices.sberbank.ru:9443/api/v2/oauth"
	restV1Base     = "https://smartspeech.sber.ru/rest/v1/"
	uploadEndpoint = restV1Base + "data:upload"
	asyncRecognize = restV1Base + "speech:async_recognize"
	taskGetPath    = restV1Base + "task:get"
	downloadPath   = restV1Base + "data:download"

	tokenCacheKey   = "access_token"
	tokenBufferSecs = 60 // refresh token this many seconds before expiry

	asyncPollInterval = 3 * time.Second
	asyncMaxWait      = 45 * time.Minute
)

// Client defines the interface for audio transcription.
type Client interface {
	Transcribe(ctx context.Context, audio []byte, mimeType string) (string, error)
}

// Config holds SaluteSpeech OAuth credentials (Authorization Key from Studio for Basic auth).
type Config struct {
	AuthorizationKey string
	Scope            string
}

// tokenResponse is the OAuth token endpoint response.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// recognizeResponse matches the JSON shape used in async download fallbacks.
type recognizeResponse struct {
	Result []struct {
		NormalizedText string `json:"normalized_text"`
	} `json:"result"`
}

// httpClient is the real SaluteSpeech REST implementation.
type httpClient struct {
	cfg        Config
	http       *http.Client
	tokenCache *gocache.Cache
	log        *logger.Logger
}

// New returns a Client that calls the SaluteSpeech REST API.
// TLS verification is disabled because SaluteSpeech uses a custom CA.
func New(cfg Config, log *logger.Logger) Client {
	return &httpClient{
		cfg: cfg,
		http: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
			},
			// No global timeout: long uploads and async polling are bounded by context / asyncMaxWait.
			Timeout: 0,
		},
		tokenCache: gocache.New(25*time.Minute, 5*time.Minute),
		log:        log.WithComponent("salutespeech"),
	}
}

// NewStub returns a Client that returns a placeholder without making any network calls.
// Use this when SaluteSpeech credentials are not configured.
func NewStub() Client { return &stubClient{} }

// Transcribe runs asynchronous recognition only: upload → async task → poll → download.
func (c *httpClient) Transcribe(ctx context.Context, audio []byte, mimeType string) (string, error) {
	token, err := c.accessToken(ctx)
	if err != nil {
		return "", fmt.Errorf("get access token: %w", err)
	}
	c.log.Info("starting async recognition", zap.Int("bytes", len(audio)))
	return c.transcribeAsync(ctx, token, audio, mimeType)
}

func (c *httpClient) authorizationHeader() (string, error) {
	raw := strings.TrimSpace(c.cfg.AuthorizationKey)
	raw = strings.ReplaceAll(raw, "\n", "")
	raw = strings.ReplaceAll(raw, "\r", "")
	if raw != "" {
		if strings.HasPrefix(strings.ToLower(raw), "basic ") {
			return raw, nil
		}
		return "Basic " + raw, nil
	}
	return "", fmt.Errorf("set salutespeech_authorization_key (Authorization Key from Studio)")
}

// accessToken returns a cached token or fetches a new one.
func (c *httpClient) accessToken(ctx context.Context) (string, error) {
	if tok, ok := c.tokenCache.Get(tokenCacheKey); ok {
		return tok.(string), nil
	}

	authHdr, err := c.authorizationHeader()
	if err != nil {
		return "", fmt.Errorf("authorization header: %w", err)
	}

	scope := strings.TrimSpace(c.cfg.Scope)
	if scope == "" {
		scope = "SALUTE_SPEECH_PERS"
	}
	form := url.Values{}
	form.Set("scope", scope)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, authEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("auth new request: %w", err)
	}
	req.Header.Set("Authorization", authHdr)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("RqUID", uuid.New().String())

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("auth request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, rerr := io.ReadAll(resp.Body)
		if rerr != nil {
			return "", fmt.Errorf("auth status %d: read body: %w", resp.StatusCode, rerr)
		}
		return "", fmt.Errorf("auth returned status %d: %s", resp.StatusCode, body)
	}

	var tok tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}

	ttl := time.Duration(tok.ExpiresIn-tokenBufferSecs) * time.Second
	if ttl < time.Minute {
		ttl = time.Minute
	}
	c.tokenCache.Set(tokenCacheKey, tok.AccessToken, ttl)

	c.log.Info("SaluteSpeech token refreshed", zap.Duration("ttl", ttl))

	return tok.AccessToken, nil
}

// --- Async recognition (upload → task → download; up to ~1 GB per SaluteSpeech docs) ---

type envelopeUpload struct {
	Status int `json:"status"`
	Result struct {
		RequestFileID string `json:"request_file_id"`
	} `json:"result"`
}

type envelopeTask struct {
	Status int `json:"status"`
	Result struct {
		ID             string `json:"id"`
		Status         string `json:"status"`
		ResponseFileID string `json:"response_file_id"`
		Error          string `json:"error"`
		ErrorMessage   string `json:"error_message"`
	} `json:"result"`
}

func (c *httpClient) transcribeAsync(ctx context.Context, token string, audio []byte, mimeType string) (string, error) {
	fileID, err := c.uploadData(ctx, token, audio)
	if err != nil {
		return "", fmt.Errorf("async upload: %w", err)
	}

	enc, rate, ch := inferAsyncAudioParams(mimeType, audio)
	c.log.Info("async recognition parameters",
		zap.String("mime_type", mimeType),
		zap.String("audio_encoding", enc),
		zap.Int("sample_rate", rate),
		zap.Int("channels", ch),
	)
	taskID, err := c.startAsyncRecognize(ctx, token, fileID, enc, rate, ch)
	if err != nil {
		return "", fmt.Errorf("async start: %w", err)
	}

	responseFileID, err := c.pollTask(ctx, token, taskID)
	if err != nil {
		return "", fmt.Errorf("async task: %w", err)
	}

	raw, err := c.downloadAsyncResult(ctx, token, responseFileID)
	if err != nil {
		return "", fmt.Errorf("async download: %w", err)
	}

	text, err := parseAsyncTranscriptJSON(raw)
	if err != nil {
		return "", fmt.Errorf("async parse result: %w", err)
	}

	c.log.Info("async transcription complete", zap.Int("chars", len(text)))
	return text, nil
}

func (c *httpClient) uploadData(ctx context.Context, token string, audio []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadEndpoint, bytes.NewReader(audio))
	if err != nil {
		return "", fmt.Errorf("upload new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Request-ID", uuid.New().String())

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("upload read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("upload status %d: %s", resp.StatusCode, body)
	}
	var env envelopeUpload
	if err := json.Unmarshal(body, &env); err != nil {
		return "", fmt.Errorf("decode upload response: %w", err)
	}
	if env.Status != http.StatusOK || env.Result.RequestFileID == "" {
		return "", fmt.Errorf("upload unexpected response: %s", body)
	}
	return env.Result.RequestFileID, nil
}

func (c *httpClient) startAsyncRecognize(ctx context.Context, token, fileID, encoding string, sampleRate, channels int) (string, error) {
	payload := map[string]any{
		"request_file_id": fileID,
		"options": map[string]any{
			"language":                   "ru-RU",
			"audio_encoding":             encoding,
			"sample_rate":                sampleRate,
			"channels_count":             channels,
			"hypotheses_count":           1,
			"enable_profanity_filter":    false,
			"max_speech_timeout":         "20s",
			"no_speech_timeout":          "7s",
			"hints":                      map[string]any{},
			"insight_models":             []any{},
			"speaker_separation_options": map[string]any{},
		},
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("async_recognize marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, asyncRecognize, bytes.NewReader(buf))
	if err != nil {
		return "", fmt.Errorf("async_recognize new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Request-ID", uuid.New().String())

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("async_recognize request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("async_recognize read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("async_recognize status %d: %s", resp.StatusCode, body)
	}
	var env envelopeTask
	if err := json.Unmarshal(body, &env); err != nil {
		return "", fmt.Errorf("decode async_recognize: %w", err)
	}
	if env.Status != http.StatusOK || env.Result.ID == "" {
		return "", fmt.Errorf("async_recognize unexpected: %s", body)
	}
	return env.Result.ID, nil
}

func (c *httpClient) pollTask(ctx context.Context, token, taskID string) (string, error) {
	deadline := time.Now().Add(asyncMaxWait)
	u, err := url.Parse(taskGetPath)
	if err != nil {
		return "", fmt.Errorf("task:get parse url: %w", err)
	}
	q := u.Query()
	q.Set("id", taskID)
	u.RawQuery = q.Encode()

	for {
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("task poll: %w", err)
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("task %s: deadline exceeded after %v", taskID, asyncMaxWait)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return "", fmt.Errorf("task:get new request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Request-ID", uuid.New().String())

		resp, err := c.http.Do(req)
		if err != nil {
			return "", fmt.Errorf("task:get request: %w", err)
		}
		body, rerr := io.ReadAll(resp.Body)
		if cerr := resp.Body.Close(); cerr != nil && rerr == nil {
			rerr = cerr
		}
		if rerr != nil {
			return "", fmt.Errorf("task:get read body: %w", rerr)
		}
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("task:get status %d: %s", resp.StatusCode, body)
		}

		var env envelopeTask
		if err := json.Unmarshal(body, &env); err != nil {
			return "", fmt.Errorf("decode task:get: %w", err)
		}
		if env.Status != http.StatusOK {
			return "", fmt.Errorf("task:get api status %d: %s", env.Status, body)
		}

		switch env.Result.Status {
		case "DONE":
			if env.Result.ResponseFileID == "" {
				return "", fmt.Errorf("task DONE but missing response_file_id")
			}
			return env.Result.ResponseFileID, nil
		case "ERROR":
			msg := env.Result.Error
			if msg == "" {
				msg = env.Result.ErrorMessage
			}
			if msg == "" {
				msg = "unknown error"
			}
			return "", fmt.Errorf("recognition task failed: %s", msg)
		case "CANCELED":
			return "", fmt.Errorf("recognition task canceled")
		default:
			// NEW, RUNNING, etc.
			time.Sleep(asyncPollInterval)
		}
	}
}

func (c *httpClient) downloadAsyncResult(ctx context.Context, token, responseFileID string) ([]byte, error) {
	u, err := url.Parse(downloadPath)
	if err != nil {
		return nil, fmt.Errorf("download parse url: %w", err)
	}
	q := u.Query()
	q.Set("response_file_id", responseFileID)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("download new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Request-ID", uuid.New().String())

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("download read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download status %d: %s", resp.StatusCode, body)
	}
	return body, nil
}

func inferAsyncAudioParams(mimeType string, audio []byte) (encoding string, sampleRate int, channels int) {
	if enc, rate, ch := opusHeadFromOGG(audio); enc != "" {
		return enc, rate, ch
	}
	m := strings.ToLower(mimeType)
	switch {
	case strings.Contains(m, "opus") || strings.Contains(m, "ogg"):
		return "OPUS", 48000, 1
	case strings.Contains(m, "mpeg") || strings.Contains(m, "mp3"):
		return "MP3", 44100, 1
	case strings.Contains(m, "flac"):
		return "FLAC", 48000, 1
	case strings.Contains(m, "wav") || strings.Contains(m, "wave"):
		return "PCM_S16LE", 16000, 1
	default:
		return "OPUS", 48000, 1
	}
}

// opusHeadFromOGG finds an OpusHead header in a buffer and returns OPUS params.
func opusHeadFromOGG(b []byte) (encoding string, sampleRate int, channels int) {
	const magic = "OpusHead"
	i := bytes.Index(b, []byte(magic))
	if i < 0 || len(b) < i+16 {
		return "", 0, 0
	}
	channels = int(b[i+9])
	if channels < 1 {
		channels = 1
	}
	rate := int(binary.LittleEndian.Uint32(b[i+12 : i+16]))
	if rate == 0 {
		rate = 48000
	}
	return "OPUS", rate, channels
}

func parseAsyncTranscriptJSON(data []byte) (string, error) {
	var items []struct {
		Results []struct {
			NormalizedText string `json:"normalized_text"`
		} `json:"results"`
	}
	if err := json.Unmarshal(data, &items); err == nil && len(items) > 0 {
		parts := make([]string, 0, len(items))
		for _, it := range items {
			if len(it.Results) == 0 {
				continue
			}
			t := strings.TrimSpace(it.Results[0].NormalizedText)
			if t != "" {
				parts = append(parts, t)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, " "), nil
		}
	}

	// Same shape as sync response
	var syncLike recognizeResponse
	if err := json.Unmarshal(data, &syncLike); err == nil && len(syncLike.Result) > 0 {
		parts := make([]string, 0, len(syncLike.Result))
		for _, r := range syncLike.Result {
			if t := strings.TrimSpace(r.NormalizedText); t != "" {
				parts = append(parts, t)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, " "), nil
		}
	}

	return "", fmt.Errorf("could not parse transcript JSON (len=%d)", len(data))
}

// stubClient returns a placeholder without any network calls.
type stubClient struct{}

func (s *stubClient) Transcribe(_ context.Context, audio []byte, _ string) (string, error) {
	return fmt.Sprintf("[SaluteSpeech not configured — file received, %d bytes]", len(audio)), nil
}
