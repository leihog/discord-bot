package lua

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// httpOptions holds parsed HTTP request options as plain Go types so they can
// be safely read on the dispatcher goroutine and then passed to a goroutine
// without touching LState again.
type httpOptions struct {
	Timeout float64
	Headers map[string]string
}

func parseHTTPOptions(options *lua.LTable) httpOptions {
	opts := httpOptions{
		Timeout: 30.0,
		Headers: make(map[string]string),
	}
	if options == nil {
		return opts
	}

	if timeoutVal := options.RawGetString("timeout"); timeoutVal != lua.LNil {
		if timeoutNum, ok := timeoutVal.(lua.LNumber); ok {
			opts.Timeout = float64(timeoutNum)
		}
	}

	if headersVal := options.RawGetString("headers"); headersVal != lua.LNil {
		if headersTbl, ok := headersVal.(*lua.LTable); ok {
			headersTbl.ForEach(func(key lua.LValue, value lua.LValue) {
				opts.Headers[key.String()] = value.String()
			})
		}
	}

	return opts
}

// doHTTPGet performs a GET request using only plain Go types. Safe to call
// from any goroutine.
func doHTTPGet(ctx context.Context, url string, opts httpOptions) HTTPResult {
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(opts.Timeout*float64(time.Second)))
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "GET", url, nil)
	if err != nil {
		return HTTPResult{Err: err}
	}
	for key, value := range opts.Headers {
		req.Header.Set(key, value)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return HTTPResult{Err: err}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return HTTPResult{Err: err}
	}

	return HTTPResult{
		StatusCode: resp.StatusCode,
		Body:       string(body),
		Headers:    resp.Header,
	}
}

// doHTTPPost performs a POST request using only plain Go types. Safe to call
// from any goroutine.
func doHTTPPost(ctx context.Context, url string, body string, opts httpOptions) HTTPResult {
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(opts.Timeout*float64(time.Second)))
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "POST", url, strings.NewReader(body))
	if err != nil {
		return HTTPResult{Err: err}
	}
	for key, value := range opts.Headers {
		req.Header.Set(key, value)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return HTTPResult{Err: err}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return HTTPResult{Err: err}
	}

	return HTTPResult{
		StatusCode: resp.StatusCode,
		Body:       string(respBody),
		Headers:    resp.Header,
	}
}

// httpGet is the synchronous Lua binding — kept for simple use cases.
func (e *Engine) httpGet(url string, options *lua.LTable) (lua.LValue, error) {
	result := doHTTPGet(context.Background(), url, parseHTTPOptions(options))
	if result.Err != nil {
		return lua.LNil, result.Err
	}
	return buildHTTPResultTable(e, result), nil
}

// httpPost is the synchronous Lua binding — kept for simple use cases.
func (e *Engine) httpPost(url string, body string, options *lua.LTable) (lua.LValue, error) {
	result := doHTTPPost(context.Background(), url, body, parseHTTPOptions(options))
	if result.Err != nil {
		return lua.LNil, result.Err
	}
	return buildHTTPResultTable(e, result), nil
}

// buildHTTPResultTable converts an HTTPResult to a Lua table.
// Must be called on the dispatcher goroutine.
func buildHTTPResultTable(e *Engine, result HTTPResult) lua.LValue {
	tbl := e.state.NewTable()
	tbl.RawSetString("status", lua.LNumber(result.StatusCode))
	tbl.RawSetString("body", lua.LString(result.Body))

	headersTable := e.state.NewTable()
	for key, values := range result.Headers {
		if len(values) > 0 {
			headersTable.RawSetString(key, lua.LString(values[0]))
		}
	}
	tbl.RawSetString("headers", headersTable)
	return tbl
}
