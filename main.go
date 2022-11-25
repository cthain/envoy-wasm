package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/tidwall/gjson"
)

func main() {
	proxywasm.SetVMContext(NewVMContext())
}

type VMContext struct {
	// Embed the default VM context here,
	// so that we don't need to reimplement all the methods.
	types.DefaultVMContext
}

func NewVMContext() *VMContext {
	proxywasm.LogDebug("creating new VMContext")
	return &VMContext{}
}

// Override types.DefaultVMContext.
func (*VMContext) NewPluginContext(contextID uint32) types.PluginContext {
	proxywasm.LogDebugf("creating new PluginContext: contextID = %d", contextID)
	return &PluginContext{contextID: contextID}
}

type PluginContext struct {
	// Embed the default plugin context here,
	// so that we don't need to reimplement all the methods.
	types.DefaultPluginContext
	contextID uint32
	config    Config
}

// Override types.DefaultPluginContext.
func (p *PluginContext) NewHttpContext(contextID uint32) types.HttpContext {
	proxywasm.LogDebugf("creating new HTTPContext: contextID = %d", contextID)
	return NewHTTPContext(&p.config)
}

// Override types.DefaultPluginContext.
func (p *PluginContext) OnPluginStart(pluginConfigurationSize int) types.OnPluginStartStatus {
	err := p.LoadConfig()
	if err != nil {
		proxywasm.LogErrorf("failed to load plugin configuration: %v", err)
		return types.OnPluginStartStatusFailed
	}
	return types.OnPluginStartStatusOK
}

type Config struct {
	SQLKeywords       []string
	RateLimitRequests int
	RateLimitInterval time.Duration
}

func (p *PluginContext) LoadConfig() error {
	proxywasm.LogDebug("loading plugin config")
	data, err := proxywasm.GetPluginConfiguration()
	if err != nil {
		return fmt.Errorf("failed to load plugin configuration: %w", err)
	}
	if data == nil {
		return fmt.Errorf("plugin configuration is nil")
	}

	if !gjson.Valid(string(data)) {
		return fmt.Errorf("plugin configuration is malformed")
	}

	// get SQL keywords
	result := gjson.Get(string(data), "sqlKeywords").Array()
	p.config.SQLKeywords = make([]string, len(result))
	for i, keyword := range result {
		p.config.SQLKeywords[i] = keyword.Str
	}

	// get rate limit config
	p.config.RateLimitRequests = int(gjson.Get(string(data), "rateLimitRequests").Int())
	s := strings.TrimSpace(gjson.Get(string(data), "rateLimitInterval").Str)
	interval, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("failed to parse rateLimitInterval: %w. ensure it is a valid time.Duration string", err)
	}
	p.config.RateLimitInterval = interval

	proxywasm.LogDebugf("plugin config: %v", p.config)
	return nil
}
