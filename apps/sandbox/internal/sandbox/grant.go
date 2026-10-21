package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	GrantFetchLimit = 64 << 20

	grantFetchTimeout = 2 * time.Minute
)

func FetchGrant(ctx context.Context, url string, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, grantFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.New("grant URL is not usable")
	}
	resp, err := GrantHTTPClient.Do(req)
	if err != nil {
		return nil, errors.New("object storage could not be reached")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("object storage answered %d", resp.StatusCode)
	}
	if resp.ContentLength > limit {
		return nil, errors.New("object exceeds the grant size limit")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, errors.New("object could not be read")
	}
	if int64(len(body)) > limit {
		return nil, errors.New("object exceeds the grant size limit")
	}
	return body, nil
}
