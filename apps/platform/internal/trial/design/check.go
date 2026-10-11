package testlab

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
)

type Check struct {
	Kind     string   `json:"kind"`
	Values   []string `json:"values,omitempty"`
	Children []Check  `json:"children,omitempty"`
	AtLeast  int      `json:"at_least,omitempty"`
}

type CheckValidator func(ctx context.Context, workspaceID pgtype.UUID, c *Check) (*Check, error)

var errChecksUnavailable = errors.New("testlab: no check validator is wired")

func (s *Service) validateCheck(ctx context.Context, workspaceID pgtype.UUID, c *Check) (*Check, error) {
	if c == nil {
		return nil, nil
	}
	if s.ValidateCheck == nil {
		return nil, errChecksUnavailable
	}
	return s.ValidateCheck(ctx, workspaceID, c)
}
