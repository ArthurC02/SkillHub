package testlab

import (
	"errors"
	"fmt"
)

type Check struct {
	Kind     string   `json:"kind"`
	Values   []string `json:"values,omitempty"`
	Children []Check  `json:"children,omitempty"`
	AtLeast  int      `json:"at_least,omitempty"`
}

type CheckValidator func(*Check) (*Check, error)

var errChecksUnavailable = errors.New("testlab: no check validator is wired")

func (s *Service) validateCheck(c *Check) (*Check, error) {
	if c == nil {
		return nil, nil
	}
	if s.ValidateCheck == nil {
		return nil, errChecksUnavailable
	}
	validated, err := s.ValidateCheck(c)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return validated, nil
}
