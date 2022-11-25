package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
)

// RateLimitFilter implements a simple HTTP rate limiter.
// It records requests from each unique IP address and counts the number
// of requests that have been received within a configurable time window.
// If the number of requests exceed a configurable limit, subsequent requests
// are blocked until the rate limit time window expires.
type RateLimitFilter struct {
	limit int
	rate  time.Duration
}

func NewRateLimitFilter(limit int, rate time.Duration) *RateLimitFilter {
	proxywasm.LogInfo("creating new RateLimitFilter")
	return &RateLimitFilter{limit: limit, rate: rate}
}

func (f *RateLimitFilter) FilterHeaders(headers map[string][]string) (int, error) {
	values, exists := headers["x-forwarded-for"]
	if !exists || values == nil {
		proxywasm.LogInfo("request missing x-forwarded-for header, rate limit filter skipped")
		return StatusOK, nil
	}

	// Get the entry for the IP address from the request. The origin IP is the first entry in the list.
	entry, err := getEntry(values[0])
	if err != nil {
		return StatusServerError, fmt.Errorf("failed to get entry: %w", err)
	}

	// Increment the request counter and calculate the delta [us] for the request window.
	entry.requests++
	delta := entry.delta()

	if entry.requests > f.limit && delta < f.rate {
		// this address has made too many requests within the rate limit window.
		return StatusTooManyRequests, fmt.Errorf("rate limit exceeded, try again later")
	}

	if delta > f.rate {
		// the rate limit interval has expired for this address, so reset the entry.
		entry.requests = 0
		entry.timestamp = entry.now
	}

	setEntry(entry)

	return StatusOK, nil
}

type entry struct {
	addr      string
	requests  int
	now       int64
	timestamp int64
	cas       uint32
}

func (e entry) delta() time.Duration {
	// Note that tinygo doesn't seem to support time.Time.Sub():
	// 	delta := now.Sub(dt)
	// 	fmt.Println(delta)
	// yields: 2562047h47m16.854775807s
	//
	// So we're using int64 representations with microsecond precision and comparing directly.
	return time.Duration(e.now-e.timestamp) * time.Microsecond
}

func getEntry(addr string) (entry, error) {
	e := entry{addr: addr, now: time.Now().UTC().UnixMicro()}

	// get the entry for the requestor from the shared data.
	data, cas, err := proxywasm.GetSharedData(addr)
	if err != nil && err != types.ErrorStatusNotFound {
		return e, fmt.Errorf("failed to get shared data for %s: %w", addr, err)
	}
	e.cas = cas

	if err == types.ErrorStatusNotFound {
		// if the entry for the address is not found, this is the first request.
		e.timestamp = e.now
	} else {
		// Tokenize the string on :
		parts := strings.Split(string(data), ":")
		if len(parts) != 2 {
			return e, fmt.Errorf("corrupt rate limit entry for %s", addr)
		}

		// Get the number of requests
		e.requests, err = strconv.Atoi(parts[0])
		if err != nil {
			return e, fmt.Errorf("failed to get count for %s: %w", addr, err)
		}

		// Get the timestamp
		e.timestamp, err = strconv.ParseInt(parts[1], 0, 64)
		if err != nil {
			return e, fmt.Errorf("failed to get time for %s: %w", addr, err)
		}
	}
	return e, nil
}

func setEntry(e entry) {
	// save the entry for this address.
	// TODO(cthain) this is thread safe because we are using the CAS id. But the whole
	// get/set logic should be in a retry loop to avoid race conditions where concurrent
	// connections are trying to access the same information at the same time; i.e.,
	// concurrent requests from the same IP address.
	err := proxywasm.SetSharedData(e.addr, []byte(fmt.Sprintf("%d:%d", e.requests, e.timestamp)), e.cas)
	if err != nil {
		// not much we can do until retries are implemented.. just log it.
		proxywasm.LogErrorf("failed to save rate limit entry for %s: %v", e.addr, err)
	}
}
