package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultBaseURL = "http://localhost:11434"

type Client struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

func New(model string) *Client {
	return &Client{
		baseURL: defaultBaseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func NewWithURL(model, baseURL string) *Client {
	c := New(model)
	c.baseURL = baseURL
	return c
}

type generateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type generateResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

func (c *Client) Generate(ctx context.Context, prompt string) (string, error) {
	body, err := json.Marshal(generateRequest{
		Model:  c.model,
		Prompt: prompt,
		Stream: false,
	})
	if err != nil {
		return "", fmt.Errorf("marshal failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("request creation failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama returned %d: %s", resp.StatusCode, string(b))
	}

	var result generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode failed: %w", err)
	}

	return result.Response, nil
}
func (c *Client) Handler() func(ctx context.Context, prompt []byte) ([]byte, string, error) {
	return func(ctx context.Context, prompt []byte) ([]byte, string, error) {
		response, err := c.Generate(ctx, string(prompt))
		if err != nil {
			return nil, "", err
		}
		return []byte(response), fmt.Sprintf("via %s", c.model), nil
	}
}

func (c *Client) WorkerHandler() func(ctx context.Context, payload []byte) ([]byte, error) {
	return func(ctx context.Context, payload []byte) ([]byte, error) {
		response, err := c.Generate(ctx, string(payload))
		if err != nil {
			return nil, err
		}
		return []byte(response), nil
	}
}