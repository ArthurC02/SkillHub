package trace

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

var (
	ErrBadToken = errors.New("invalid trace ingestion token")

	ErrTokenExpired = errors.New("trace ingestion token has expired")
)

type Grant struct {
	RunID   pgtype.UUID
	Attempt int
}

type Signer struct {
	Secret []byte

	TTL time.Duration
}

const DefaultTTL = 2 * time.Hour

func (s *Signer) Enabled() bool { return s != nil && len(s.Secret) > 0 }

func (s *Signer) ttl() time.Duration {
	if s.TTL > 0 {
		return s.TTL
	}
	return DefaultTTL
}

func (s *Signer) Mint(runID pgtype.UUID, attempt int, now time.Time) string {
	if !s.Enabled() {
		return ""
	}
	body := fmt.Sprintf("%s.%d.%d", pgconv.UUIDString(runID), attempt, now.Add(s.ttl()).Unix())
	return body + "." + s.sign(body)
}

func (s *Signer) IngestionURL(base string, runID pgtype.UUID, attempt int, now time.Time) string {
	token := s.Mint(runID, attempt, now)
	if base == "" || token == "" {
		return ""
	}
	return strings.TrimSuffix(base, "/") + IngestPath + token
}

const IngestPath = "/internal/trace/"

func (s *Signer) Verify(token string, now time.Time) (Grant, error) {
	if !s.Enabled() {
		return Grant{}, ErrBadToken
	}

	// Split on the last dot: the signed body itself contains dots.
	i := strings.LastIndex(token, ".")
	if i <= 0 || i == len(token)-1 {
		return Grant{}, ErrBadToken
	}
	body, sig := token[:i], token[i+1:]
	if !hmac.Equal([]byte(sig), []byte(s.sign(body))) {
		return Grant{}, ErrBadToken
	}

	parts := strings.Split(body, ".")
	if len(parts) != 3 {
		return Grant{}, ErrBadToken
	}
	var runID pgtype.UUID
	if err := runID.Scan(parts[0]); err != nil {
		return Grant{}, ErrBadToken
	}
	attempt, err := strconv.Atoi(parts[1])
	if err != nil || attempt < 1 {
		return Grant{}, ErrBadToken
	}
	exp, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return Grant{}, ErrBadToken
	}
	if now.After(time.Unix(exp, 0)) {
		return Grant{}, ErrTokenExpired
	}
	return Grant{RunID: runID, Attempt: attempt}, nil
}

func (s *Signer) sign(body string) string {
	mac := hmac.New(sha256.New, s.Secret)
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}
