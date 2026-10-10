package testlab

import (
	"fmt"
	"strings"
)

type Check struct {
	Kind   string   `json:"kind"`
	Values []string `json:"values,omitempty"`
}

const (
	CheckContainsValues = "contains_values"
	CheckAsks           = "asks"
	CheckDoesNotAsk     = "does_not_ask"
)

const (
	MaxCheckValues     = 20
	MaxCheckValueBytes = 200
)

type valueCount struct{ min, max int }

var checkValueCounts = map[string]valueCount{
	CheckContainsValues: {min: 1, max: MaxCheckValues},
	CheckAsks:           {min: 0, max: MaxCheckValues},
	CheckDoesNotAsk:     {min: 0, max: 0},
}

func validateCheck(c *Check) (*Check, error) {
	if c == nil {
		return nil, nil
	}
	count, known := checkValueCounts[c.Kind]
	if !known {
		return nil, fmt.Errorf("%w: 不認得的檢查方式 %q", ErrInvalid, c.Kind)
	}
	values := make([]string, 0, len(c.Values))
	for _, v := range c.Values {
		v = strings.TrimSpace(v)
		if v == "" {
			return nil, fmt.Errorf("%w: 檢查的值不能空白", ErrInvalid)
		}
		if len(v) > MaxCheckValueBytes {
			return nil, fmt.Errorf("%w: 每個檢查的值最多 %d bytes", ErrInvalid, MaxCheckValueBytes)
		}
		values = append(values, v)
	}
	if len(values) < count.min || len(values) > count.max {
		return nil, fmt.Errorf("%w: %s 的值要 %d 到 %d 個", ErrInvalid, c.Kind, count.min, count.max)
	}
	if len(values) == 0 {
		values = nil
	}
	return &Check{Kind: c.Kind, Values: values}, nil
}
