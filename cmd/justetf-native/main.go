package main

/*
#include <stdint.h>
#include <stdlib.h>
typedef struct {
    void *data;
    size_t length;
} JustETFBuffer;
*/
import "C"

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"
	"unsafe"

	justetf "github.com/wavepacket0/justetf-mobile-client"
)

const (
	statusOK = iota
	statusInvalidArgument
	statusAPI
	statusTransport
	statusClosed
	statusInternal
	statusAuthentication
)

const (
	maxOperationBytes  = 64
	maxNativeJSONBytes = 64 << 10
)

type session struct {
	client *justetf.Client
	ctx    context.Context
	cancel context.CancelFunc
}

var sessions = struct {
	sync.RWMutex
	next    uint64
	clients map[uint64]*session
}{clients: make(map[uint64]*session)}

func storeClient(client *justetf.Client) uint64 {
	ctx, cancel := context.WithCancel(context.Background())
	sessions.Lock()
	defer sessions.Unlock()
	if sessions.next == ^uint64(0) {
		cancel()
		panic("client handles exhausted")
	}
	sessions.next++
	sessions.clients[sessions.next] = &session{client: client, ctx: ctx, cancel: cancel}
	return sessions.next
}

func findClient(handle uint64) *session {
	sessions.RLock()
	defer sessions.RUnlock()
	return sessions.clients[handle]
}

func closeClient(handle uint64) bool {
	sessions.Lock()
	current, exists := sessions.clients[handle]
	delete(sessions.clients, handle)
	sessions.Unlock()
	if exists {
		current.client.Close()
		current.cancel()
	}
	return exists
}

func writeBuffer(output *C.JustETFBuffer, data []byte) {
	if len(data) == 0 {
		*output = C.JustETFBuffer{}
		return
	}
	output.data = C.CBytes(data)
	output.length = C.size_t(len(data))
}

func fail(output *C.JustETFBuffer, code int, message string, httpStatus int) C.int {
	if output != nil {
		body, _ := json.Marshal(struct {
			Message string `json:"message"`
			Status  int    `json:"status,omitempty"`
		}{message, httpStatus})
		writeBuffer(output, body)
	}
	return C.int(code)
}

func guard(output *C.JustETFBuffer, status *C.int) {
	if recover() != nil {
		if output != nil && output.data != nil {
			C.free(output.data)
			*output = C.JustETFBuffer{}
		}
		*status = fail(output, statusInternal, "internal native client error", 0)
	}
}

//export justetf_abi_version
func justetf_abi_version() C.uint32_t {
	return 1
}

//export justetf_client_new
func justetf_client_new(configData *C.char, configLength C.size_t, handle *C.uint64_t, output *C.JustETFBuffer) (status C.int) {
	defer guard(output, &status)
	if output == nil {
		return statusInvalidArgument
	}
	*output = C.JustETFBuffer{}
	if handle == nil {
		return fail(output, statusInvalidArgument, "missing client handle output", 0)
	}
	*handle = 0
	if configData == nil || configLength == 0 || configLength > maxNativeJSONBytes {
		return fail(output, statusInvalidArgument, "invalid client configuration length", 0)
	}
	var config struct {
		BaseURL   string `json:"base_url"`
		APIKey    string `json:"api_key"`
		Language  string `json:"language"`
		Country   string `json:"country"`
		Currency  string `json:"currency"`
		TimeoutMS int64  `json:"timeout_ms"`
	}
	if err := json.Unmarshal(C.GoBytes(unsafe.Pointer(configData), C.int(configLength)), &config); err != nil {
		return fail(output, statusInvalidArgument, "invalid client configuration JSON", 0)
	}
	if config.TimeoutMS < 0 || config.TimeoutMS > 300000 {
		return fail(output, statusInvalidArgument, "timeout_ms must be between 0 (default) and 300000", 0)
	}
	client, err := justetf.NewClient(justetf.Config{
		BaseURL: config.BaseURL, APIKey: config.APIKey, Language: config.Language,
		Country: config.Country, Currency: config.Currency,
		Timeout: time.Duration(config.TimeoutMS) * time.Millisecond,
	})
	if err != nil {
		return fail(output, statusInvalidArgument, err.Error(), 0)
	}
	*handle = C.uint64_t(storeClient(client))
	return statusOK
}

//export justetf_profile
func justetf_profile(handle C.uint64_t, isinData *C.char, isinLength C.size_t, output *C.JustETFBuffer) (status C.int) {
	defer guard(output, &status)
	if output == nil {
		return statusInvalidArgument
	}
	*output = C.JustETFBuffer{}
	current := findClient(uint64(handle))
	if current == nil {
		return fail(output, statusClosed, "client is closed or handle is invalid", 0)
	}
	if isinData == nil || isinLength != 12 {
		return fail(output, statusInvalidArgument, justetf.ErrInvalidISIN.Error(), 0)
	}
	isin := C.GoStringN(isinData, C.int(isinLength))
	body, err := current.client.GetProfile(current.ctx, isin)
	return finishResult(output, body, err)
}

func finishResult(output *C.JustETFBuffer, body []byte, err error) C.int {
	if err != nil {
		var apiError *justetf.APIError
		switch {
		case errors.Is(err, justetf.ErrInvalidISIN), errors.Is(err, justetf.ErrInvalidOptions), errors.Is(err, errInvalidArguments):
			return fail(output, statusInvalidArgument, err.Error(), 0)
		case errors.Is(err, justetf.ErrClientClosed), errors.Is(err, context.Canceled):
			return fail(output, statusClosed, "client is closed", 0)
		case errors.Is(err, justetf.ErrAuthenticationRequired), errors.Is(err, justetf.ErrSessionChanged):
			return fail(output, statusAuthentication, err.Error(), 0)
		case errors.As(err, &apiError):
			return fail(output, statusAPI, err.Error(), apiError.StatusCode)
		default:
			return fail(output, statusTransport, err.Error(), 0)
		}
	}
	writeBuffer(output, body)
	return statusOK
}

//export justetf_call
func justetf_call(handle C.uint64_t, operationData *C.char, operationLength C.size_t, paramsData *C.char, paramsLength C.size_t, output *C.JustETFBuffer) (status C.int) {
	defer guard(output, &status)
	if output == nil {
		return statusInvalidArgument
	}
	*output = C.JustETFBuffer{}
	current := findClient(uint64(handle))
	if current == nil {
		return fail(output, statusClosed, "client is closed or handle is invalid", 0)
	}
	if operationData == nil || operationLength == 0 || operationLength > maxOperationBytes {
		return fail(output, statusInvalidArgument, "invalid operation name length", 0)
	}
	if paramsData == nil || paramsLength == 0 || paramsLength > maxNativeJSONBytes {
		return fail(output, statusInvalidArgument, "invalid operation parameters length", 0)
	}
	operation := C.GoStringN(operationData, C.int(operationLength))
	parameters := C.GoBytes(unsafe.Pointer(paramsData), C.int(paramsLength))
	body, err := dispatch(current.ctx, current.client, operation, parameters)
	return finishResult(output, body, err)
}

//export justetf_client_close
func justetf_client_close(handle C.uint64_t) (status C.int) {
	defer guard(nil, &status)
	if !closeClient(uint64(handle)) {
		return statusClosed
	}
	return statusOK
}

//export justetf_buffer_free
func justetf_buffer_free(buffer unsafe.Pointer) {
	C.free(buffer)
}

func main() {}
