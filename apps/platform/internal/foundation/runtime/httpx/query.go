package httpx

import (
	"fmt"
	"net/http"
	"strconv"
)

func QueryValue(r *http.Request, name string) (value string, present bool, err error) {
	values, present := r.URL.Query()[name]
	if !present {
		return "", false, nil
	}
	if len(values) != 1 {
		return "", true, fmt.Errorf("query parameter %s must be given once", name)
	}
	return values[0], true, nil
}

func QueryInt32(r *http.Request, name string, fallback, low, high int64) (int32, error) {
	raw, present, err := QueryValue(r, name)
	if err != nil {
		return 0, err
	}
	if !present {
		return int32(fallback), nil
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || n < low || n > high {
		return 0, fmt.Errorf("query parameter %s must be a whole number between %d and %d", name, low, high)
	}
	return int32(n), nil
}
