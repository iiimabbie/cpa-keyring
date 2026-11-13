package main

/*
#include <stdint.h>
#include <stdlib.h>
typedef struct { void* ptr; size_t len; } cliproxy_buffer;
typedef struct { uint32_t abi_version; void* host_ctx; void* call; void* free_buffer; } cliproxy_host_api;
typedef int (*call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*free_fn)(void*, size_t);
typedef void (*shutdown_fn)(void);
typedef struct { uint32_t abi_version; call_fn call; free_fn free_buffer; shutdown_fn shutdown; } cliproxy_plugin_api;
extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);
*/
import "C"
import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"unsafe"
)

//export cliproxy_plugin_init
func cliproxy_plugin_init(_ *C.cliproxy_host_api, p *C.cliproxy_plugin_api) C.int {
	if p == nil {
		return 1
	}
	p.abi_version = 1
	p.call = C.call_fn(C.cliproxyPluginCall)
	p.free_buffer = C.free_fn(C.cliproxyPluginFree)
	p.shutdown = C.shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, req *C.uint8_t, n C.size_t, out *C.cliproxy_buffer) C.int {
	if method == nil || out == nil {
		return 1
	}
	out.ptr = nil
	out.len = 0
	var raw []byte
	if req != nil && n > 0 {
		raw = C.GoBytes(unsafe.Pointer(req), C.int(n))
	}
	result, err := dispatch(C.GoString(method), raw)
	var envelope any
	if err != nil {
		log.Printf("keyring: %v", err)
		envelope = map[string]any{"ok": false, "error": map[string]string{"code": "keyring_error", "message": err.Error()}}
	} else {
		envelope = map[string]any{"ok": true, "result": result}
	}
	b, e := json.Marshal(envelope)
	if e != nil {
		return 1
	}
	out.ptr = C.CBytes(b)
	out.len = C.size_t(len(b))
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, _ C.size_t) { C.free(ptr) }

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {}
func main()                   {}
func dispatch(method string, raw []byte) (any, error) {
	switch method {
	case "plugin.register", "plugin.reconfigure":
		if err := configure(raw); err != nil {
			return nil, err
		}
		return map[string]any{"schema_version": 6, "metadata": map[string]any{"Name": "keyring", "Version": version, "Author": "iiimabbie", "GitHubRepository": "https://github.com/iiimabbie/cpa-keyring", "ConfigFields": []any{map[string]any{"Name": "keys", "Type": "array", "Description": "Per caller scope rules"}}}, "capabilities": map[string]bool{"scheduler": true, "scheduler_across_priorities": true, "request_interceptor": true, "response_interceptor": true, "management_api": true}}, nil
	case "scheduler.pick":
		var req struct {
			Options    struct{ Metadata map[string]any }
			Candidates []candidate
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, err
		}
		s, _ := req.Options.Metadata["caller_scope"].(string)
		if r, ok := policy(s); ok {
			return pick(r, req.Candidates), nil
		}
		return map[string]any{"Handled": false}, nil
	case "request.intercept_before":
		var req struct{ CallerScope, Model, RequestedModel string }
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, err
		}
		model := req.RequestedModel
		if model == "" {
			model = req.Model
		}
		if r, ok := policy(req.CallerScope); ok && !allowed(r, model) {
			b, _ := json.Marshal(map[string]any{"error": map[string]any{"message": fmt.Sprintf("keyring: model %q is not allowed", model), "type": "permission_error", "code": "model_not_allowed"}})
			return map[string]any{"Terminate": true, "StatusCode": 403, "ResponseHeaders": http.Header{"Content-Type": []string{"application/json"}}, "ResponseBody": b}, nil
		}
		return map[string]any{}, nil
	case "request.intercept_after":
		return map[string]any{}, nil
	case "response.intercept_after":
		var req struct {
			Model, RequestedModel string
			RequestHeaders        http.Header
			Metadata              map[string]any
			Body                  []byte
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, err
		}
		if req.Model != "" || req.RequestedModel != "" {
			return map[string]any{}, nil
		}
		scope, _ := req.Metadata["caller_scope"].(string)
		if scope == "" {
			scope = headersScope(req.RequestHeaders)
		}
		if r, ok := policy(scope); ok {
			body, valid, err := filterList(req.Body, r)
			if err != nil {
				return nil, err
			}
			if valid {
				return map[string]any{"Body": body}, nil
			}
		}
		return map[string]any{}, nil
	case "management.register":
		return map[string]any{"Resources": []any{map[string]any{"Path": "/open", "Menu": "Keyring", "Description": "Manage client key policies"}}}, nil
	case "management.handle":
		return map[string]any{"StatusCode": 200, "Headers": http.Header{"Content-Type": []string{"text/html; charset=utf-8"}, "Cache-Control": []string{"no-store"}}, "Body": []byte(page)}, nil
	default:
		return nil, fmt.Errorf("unsupported method %s", method)
	}
}
