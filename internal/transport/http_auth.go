package transport

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mgate-agent/internal/auth"
)

func BuildHTTPURL(baseURL, path string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse cloud.base_url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("cloud.base_url must use http or https")
	}
	if u.Host == "" {
		return "", fmt.Errorf("cloud.base_url must include host")
	}
	pathURL, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("parse path: %w", err)
	}
	if pathURL.IsAbs() || !strings.HasPrefix(path, "/") {
		return "", fmt.Errorf("path must be an absolute path")
	}
	u.Path = path
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func signedHeader(secret []byte, deviceID, tenantID, agentVersion, method, path string, body []byte, now time.Time) (http.Header, error) {
	nonce, err := randomNonce()
	if err != nil {
		return nil, err
	}
	timestamp := now.UTC().Format(time.RFC3339)
	signature := auth.Sign(secret, auth.SignInput{
		Method:    method,
		Path:      path,
		Timestamp: timestamp,
		Nonce:     nonce,
		Body:      body,
	})
	header := http.Header{}
	// HMAC 认证材料必须放 header。URL 往往会进入代理日志，不能承载 secret 派生值。
	header.Set("X-MGate-Device-ID", deviceID)
	header.Set("X-MGate-Tenant-ID", tenantID)
	header.Set("X-MGate-Timestamp", timestamp)
	header.Set("X-MGate-Nonce", nonce)
	header.Set("X-MGate-Signature", signature)
	header.Set("X-MGate-Agent-Version", agentVersion)
	return header, nil
}

func newSignedJSONRequest(ctx context.Context, method, rawURL, path string, body []byte, opts PullClientOptions) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	header, err := signedHeader(opts.DeviceSecret, opts.DeviceID, opts.TenantID, opts.AgentVersion, method, path, body, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	for key, values := range header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}
