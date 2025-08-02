package lua

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// httpGet performs an HTTP GET request
func (e *Engine) httpGet(url string, options *lua.LTable) (lua.LValue, error) {
	// Parse options
	timeout := 30.0 // default 30 seconds
	headers := make(map[string]string)

	if options != nil {
		// Get timeout
		if timeoutVal := options.RawGetString("timeout"); timeoutVal != lua.LNil {
			if timeoutNum, ok := timeoutVal.(lua.LNumber); ok {
				timeout = float64(timeoutNum)
			}
		}

		// Get headers
		if headersTable := options.RawGetString("headers"); headersTable != lua.LNil {
			if headersTbl, ok := headersTable.(*lua.LTable); ok {
				headersTbl.ForEach(func(key lua.LValue, value lua.LValue) {
					headers[key.String()] = value.String()
				})
			}
		}
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout*float64(time.Second)))
	defer cancel()

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return lua.LNil, err
	}

	// Add headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Perform request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return lua.LNil, err
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return lua.LNil, err
	}

	// Create response table
	result := e.state.NewTable()
	result.RawSetString("status", lua.LNumber(resp.StatusCode))
	result.RawSetString("body", lua.LString(string(body)))

	// Convert headers to Lua table
	headersTable := e.state.NewTable()
	for key, values := range resp.Header {
		if len(values) > 0 {
			headersTable.RawSetString(key, lua.LString(values[0]))
		}
	}
	result.RawSetString("headers", headersTable)

	return result, nil
}

// httpPost performs an HTTP POST request
func (e *Engine) httpPost(url string, body string, options *lua.LTable) (lua.LValue, error) {
	// Parse options
	timeout := 30.0 // default 30 seconds
	headers := make(map[string]string)

	if options != nil {
		// Get timeout
		if timeoutVal := options.RawGetString("timeout"); timeoutVal != lua.LNil {
			if timeoutNum, ok := timeoutVal.(lua.LNumber); ok {
				timeout = float64(timeoutNum)
			}
		}

		// Get headers
		if headersTable := options.RawGetString("headers"); headersTable != lua.LNil {
			if headersTbl, ok := headersTable.(*lua.LTable); ok {
				headersTbl.ForEach(func(key lua.LValue, value lua.LValue) {
					headers[key.String()] = value.String()
				})
			}
		}
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout*float64(time.Second)))
	defer cancel()

	// Create request with body
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(body))
	if err != nil {
		return lua.LNil, err
	}

	// Add headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Perform request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return lua.LNil, err
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return lua.LNil, err
	}

	// Create response table
	result := e.state.NewTable()
	result.RawSetString("status", lua.LNumber(resp.StatusCode))
	result.RawSetString("body", lua.LString(string(respBody)))

	// Convert headers to Lua table
	headersTable := e.state.NewTable()
	for key, values := range resp.Header {
		if len(values) > 0 {
			headersTable.RawSetString(key, lua.LString(values[0]))
		}
	}
	result.RawSetString("headers", headersTable)

	return result, nil
}
