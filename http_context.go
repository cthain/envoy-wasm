package main

import (
	"fmt"
	"strings"

	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
)

const (
	StatusOK              = 200
	StatusBadRequest      = 400
	StatusTooManyRequests = 429
	StatusServerError     = 500
)

type HeaderFilter interface {
	FilterHeaders(headers map[string][]string) (int, error)
}

type BodyFilter interface {
	FilterBody(body []byte) (int, error)
}

type HTTPContext struct {
	types.DefaultHttpContext
	hFilters []HeaderFilter
	bFilters []BodyFilter
}

func NewHTTPContext(config *Config) *HTTPContext {
	httpCtx := &HTTPContext{}
	rlFilter := NewRateLimitFilter(config.RateLimitRequests, config.RateLimitInterval)
	sqlFilter := NewSQLFilter(config.SQLKeywords)

	return httpCtx.WithHeaderFilters(rlFilter, sqlFilter).WithBodyFilters(sqlFilter)
}

func (h *HTTPContext) WithBodyFilters(f ...BodyFilter) *HTTPContext {
	h.bFilters = append(h.bFilters, f...)
	return h
}

func (h *HTTPContext) WithHeaderFilters(f ...HeaderFilter) *HTTPContext {
	h.hFilters = append(h.hFilters, f...)
	return h
}

// OnHttpRequestHeaders is called when HTTP headers are received.
// The HTTPContext calls each registered HeaderFilter with the headers.
// If all filters return successfully the request continues. If any filter
// returns with an error a response is immediately returned to the
// downstream client and the request is terminated.
func (h *HTTPContext) OnHttpRequestHeaders(_ int, _ bool) types.Action {
	headers, err := GetHTTPRequestHeaders()
	if err != nil {
		proxywasm.LogErrorf("failed to get request headers: %v", err)
		return types.ActionContinue
	}

	for _, f := range h.hFilters {
		status, err := f.FilterHeaders(headers)
		if err != nil {
			proxywasm.LogInfo(err.Error())
			err = proxywasm.SendHttpResponse(uint32(status), nil, []byte(err.Error()), -1)
			if err != nil {
				proxywasm.LogErrorf("failed to send HTTP response: %v", err)
			}
			return types.ActionPause
		}
	}

	return types.ActionContinue
}

// OnHttpRequestBody is called when an HTTP request body is received.
// The HTTPContext calls each registered BodyFilter with the body content.
// If all filters return successfully the request continues. If any filter
// returns with an error a response is immediately returned to the
// downstream client and the request is terminated.
func (h *HTTPContext) OnHttpRequestBody(size int, eos bool) types.Action {

	proxywasm.LogDebugf("OnHttpRequestBody called: size = %d, eos = %t", size, eos)

	if !eos {
		// If we haven't reached the end of the stream then return ActionPause until
		// we get all the data. We buffer it so that we don't stream the partial
		// data to the upstream until we have inspected the entire contents of the
		// body.
		return types.ActionPause
	}

	body, err := proxywasm.GetHttpRequestBody(0, size)
	if err != nil {
		proxywasm.LogErrorf("failed to get request body: %v", err)
		err = proxywasm.SendHttpResponse(uint32(StatusServerError), nil, []byte(err.Error()), -1)
		if err != nil {
			proxywasm.LogErrorf("failed to read request body: %v", err)
		}
		return types.ActionPause
	}

	for _, f := range h.bFilters {
		status, err := f.FilterBody(body)
		if err != nil {
			proxywasm.LogInfo(err.Error())
			err = proxywasm.SendHttpResponse(uint32(status), nil, []byte(err.Error()), -1)
			if err != nil {
				proxywasm.LogErrorf("failed to send HTTP response: %v", err)
			}
			return types.ActionPause
		}
	}

	return types.ActionContinue
}

func GetHTTPRequestHeaders() (map[string][]string, error) {
	headers := make(map[string][]string)
	rawHeaders, err := proxywasm.GetHttpRequestHeaders()
	if err != nil {
		return headers, fmt.Errorf("failed to get HTTP headers: %w", err)
	}

	for _, header := range rawHeaders {
		rawValues := strings.Split(header[1], ",")
		var values []string
		if v, exists := headers[header[0]]; exists {
			values = v
		} else {
			values = make([]string, 0, len(rawValues))
		}
		headers[header[0]] = append(values, rawValues...)
	}
	return headers, nil
}

func Retry(attempts int, f func() error) error {
	var err error
	var n int
	for attempts > 0 && n < attempts {
		err = f()
		if err == nil {
			return nil
		}
	}
	return err
}
