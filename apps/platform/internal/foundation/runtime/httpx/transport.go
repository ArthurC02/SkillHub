package httpx

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
)

type Transport struct {
	Client        *http.Client
	Token         string
	ResponseLimit int64
}

func (t Transport) Do(ctx context.Context, method, endpoint string, payload []byte) (int, []byte, error) {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return 0, nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if t.Token != "" {
		req.Header.Set("Authorization", "Bearer "+t.Token)
	}
	client := t.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	reader := io.Reader(resp.Body)
	if t.ResponseLimit > 0 {
		reader = io.LimitReader(reader, t.ResponseLimit+1)
	}
	response, err := io.ReadAll(reader)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	if t.ResponseLimit > 0 && int64(len(response)) > t.ResponseLimit {
		return resp.StatusCode, nil, fmt.Errorf("response exceeds %d bytes", t.ResponseLimit)
	}
	return resp.StatusCode, response, err
}
