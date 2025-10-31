package protocol

import (
	"encoding/json"
	"testing"
)

func TestJSONRPCRequest_Marshal(t *testing.T) {
	tests := []struct {
		name    string
		request JSONRPCRequest
		want    string
	}{
		{
			name: "basic request with string ID",
			request: JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      "test-id",
				Method:  "test_method",
				Params:  json.RawMessage(`{"key":"value"}`),
			},
			want: `{"jsonrpc":"2.0","id":"test-id","method":"test_method","params":{"key":"value"}}`,
		},
		{
			name: "request with integer ID",
			request: JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      float64(123),
				Method:  "test_method",
			},
			want: `{"jsonrpc":"2.0","id":123,"method":"test_method"}`,
		},
		{
			name: "notification without ID",
			request: JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "notification_method",
			},
			want: `{"jsonrpc":"2.0","method":"notification_method"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.request)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}

			// Unmarshal both to compare as objects (avoid whitespace issues)
			var gotObj, wantObj map[string]interface{}
			if err := json.Unmarshal(got, &gotObj); err != nil {
				t.Fatalf("Unmarshal got error = %v", err)
			}
			if err := json.Unmarshal([]byte(tt.want), &wantObj); err != nil {
				t.Fatalf("Unmarshal want error = %v", err)
			}

			gotJSON, _ := json.Marshal(gotObj)
			wantJSON, _ := json.Marshal(wantObj)

			if string(gotJSON) != string(wantJSON) {
				t.Errorf("Marshal() = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

func TestJSONRPCRequest_Unmarshal(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    JSONRPCRequest
		wantErr bool
	}{
		{
			name:  "valid request",
			input: `{"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersion":"2025-03-26"}}`,
			want: JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      "1",
				Method:  "initialize",
				Params:  json.RawMessage(`{"protocolVersion":"2025-03-26"}`),
			},
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			input:   `{invalid}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got JSONRPCRequest
			err := json.Unmarshal([]byte(tt.input), &got)

			if (err != nil) != tt.wantErr {
				t.Errorf("Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if got.JSONRPC != tt.want.JSONRPC {
					t.Errorf("JSONRPC = %v, want %v", got.JSONRPC, tt.want.JSONRPC)
				}
				if got.Method != tt.want.Method {
					t.Errorf("Method = %v, want %v", got.Method, tt.want.Method)
				}
			}
		})
	}
}

func TestJSONRPCResponse_Marshal(t *testing.T) {
	tests := []struct {
		name     string
		response JSONRPCResponse
		wantErr  bool
	}{
		{
			name: "success response",
			response: JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      "1",
				Result:  json.RawMessage(`{"status":"ok"}`),
			},
			wantErr: false,
		},
		{
			name: "error response",
			response: JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      "1",
				Error: &JSONRPCError{
					Code:    -32600,
					Message: "Invalid Request",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.response)
			if (err != nil) != tt.wantErr {
				t.Errorf("Marshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(got) == 0 {
				t.Error("Marshal() returned empty result")
			}
		})
	}
}

func TestNewJSONRPCError(t *testing.T) {
	tests := []struct {
		name    string
		code    int
		message string
		data    interface{}
	}{
		{
			name:    "simple error",
			code:    -32600,
			message: "Invalid Request",
			data:    nil,
		},
		{
			name:    "error with data",
			code:    -32603,
			message: "Internal error",
			data:    map[string]string{"detail": "something went wrong"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewJSONRPCError(tt.code, tt.message, tt.data)

			if err.Code != tt.code {
				t.Errorf("Code = %v, want %v", err.Code, tt.code)
			}
			if err.Message != tt.message {
				t.Errorf("Message = %v, want %v", err.Message, tt.message)
			}
		})
	}
}

func TestNewTextContent(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{
			name: "simple text",
			text: "Hello, world!",
		},
		{
			name: "empty text",
			text: "",
		},
		{
			name: "multiline text",
			text: "Line 1\nLine 2\nLine 3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := NewTextContent(tt.text)

			if content.Type != "text" {
				t.Errorf("Type = %v, want text", content.Type)
			}
			if content.Text != tt.text {
				t.Errorf("Text = %v, want %v", content.Text, tt.text)
			}
		})
	}
}

func TestTool_Validation(t *testing.T) {
	tests := []struct {
		name    string
		tool    Tool
		wantErr bool
	}{
		{
			name: "valid tool",
			tool: Tool{
				Name:        "test_tool",
				Description: "A test tool",
				InputSchema: InputSchema{
					Type: "object",
					Properties: map[string]Property{
						"param1": {
							Type:        "string",
							Description: "A string parameter",
						},
					},
					Required: []string{"param1"},
				},
			},
			wantErr: false,
		},
		{
			name: "tool without name",
			tool: Tool{
				Description: "A test tool",
				InputSchema: InputSchema{Type: "object"},
			},
			wantErr: false, // Name is not validated during construction
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.tool)
			if (err != nil) != tt.wantErr {
				t.Errorf("Marshal() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(data) == 0 {
				t.Error("Marshal() returned empty result")
			}
		})
	}
}

func TestErrorCodes(t *testing.T) {
	// Test that error codes match JSON-RPC specification
	tests := []struct {
		name string
		code int
		want int
	}{
		{"ParseError", ErrCodeParseError, -32700},
		{"InvalidRequest", ErrCodeInvalidRequest, -32600},
		{"MethodNotFound", ErrCodeMethodNotFound, -32601},
		{"InvalidParams", ErrCodeInvalidParams, -32602},
		{"InternalError", ErrCodeInternalError, -32603},
		{"ToolNotFound", ErrCodeToolNotFound, -32001},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.code != tt.want {
				t.Errorf("Error code %s = %d, want %d", tt.name, tt.code, tt.want)
			}
		})
	}
}
