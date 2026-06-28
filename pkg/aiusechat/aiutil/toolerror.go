package aiutil

import "encoding/json"

type ToolError struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Details json.RawMessage `json:"details,omitempty"`
}

func NewToolError(code, message string, details any) *ToolError {
	detailsBytes, _ := json.Marshal(details)
	return &ToolError{
		Code:    code,
		Message: message,
		Details: detailsBytes,
	}
}

func (e *ToolError) Error() string {
	return e.Message
}