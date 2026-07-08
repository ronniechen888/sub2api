package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/google/uuid"
)

func main() {
	client, err := antigravity.NewClient("socks5://127.0.0.1:1080")
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	refreshToken := strings.TrimSpace(os.Getenv("ANTIGRAVITY_REFRESH_TOKEN"))
	if refreshToken == "" {
		log.Fatal("ANTIGRAVITY_REFRESH_TOKEN is required")
	}
	tokenResp, err := client.RefreshToken(context.Background(), refreshToken)
	if err != nil {
		log.Fatalf("failed to refresh token: %v", err)
	}

	accessToken := tokenResp.AccessToken
	projectID := "distributed-bastion-04x0w"

	geminiReq := map[string]any{
		"contents": []any{
			map[string]any{
				"role": "user",
				"parts": []any{
					map[string]any{
						"text": "a cute kitten playing with yarn",
					},
				},
			},
		},
		"generationConfig": map[string]any{
			"imageConfig": map[string]any{
				"aspectRatio": "1:1",
				"imageSize":   "1024x1024",
			},
		},
	}

	wrapped := map[string]any{
		"project":     projectID,
		"requestId":   "agent-" + uuid.New().String(),
		"userAgent":   "antigravity",
		"requestType": "agent",
		"model":       "gemini-3.1-flash-image",
		"request":     geminiReq,
	}

	bodyBytes, _ := json.Marshal(wrapped)

	// Send to v1internal:generateContent
	apiURL := "https://cloudcode-pa.googleapis.com/v1internal:generateContent"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		log.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", "antigravity/3.13.0 windows/amd64")

	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("failed to read response: %v", err)
	}

	fmt.Printf("HTTP Status: %d\n", resp.StatusCode)
	fmt.Printf("Response Body: %s\n", string(respBytes))
}
