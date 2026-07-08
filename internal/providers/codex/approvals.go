package codex

import (
	"encoding/json"

	"abolqasem/internal/workspace/transcript"
)

type ServerRequest struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type ServerResponse struct {
	ID     string         `json:"id"`
	Result map[string]any `json:"result,omitempty"`
	Error  map[string]any `json:"error,omitempty"`
}

type ToolRequest struct {
	Tool map[string]any
}

type ApprovalRequest struct {
	RequestID string
	Kind      string
	Params    json.RawMessage
}

type RequestHandlers struct {
	OnToolRequest     func(ToolRequest) (map[string]any, error)
	OnApprovalRequest func(ApprovalRequest) (string, error)
}

func HandleServerRequest(request ServerRequest, handlers RequestHandlers) ([]HarnessEvent, ServerResponse, error) {
	switch request.Method {
	case "item/tool/requestUserInput":
		return handleUserInputRequest(request, handlers)
	case "item/commandExecution/requestApproval":
		return handleApprovalRequest(request, handlers, "command_execution")
	case "item/fileChange/requestApproval":
		return handleApprovalRequest(request, handlers, "file_change")
	default:
		return nil, ServerResponse{
			ID:    request.ID,
			Error: map[string]any{"message": "Unsupported Codex server request"},
		}, nil
	}
}

func handleUserInputRequest(request ServerRequest, handlers RequestHandlers) ([]HarnessEvent, ServerResponse, error) {
	var params struct {
		ItemID    string `json:"itemId"`
		Questions []struct {
			ID       string `json:"id"`
			Header   string `json:"header"`
			Question string `json:"question"`
			Options  []struct {
				Label       string `json:"label"`
				Description string `json:"description,omitempty"`
			} `json:"options"`
		} `json:"questions"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return nil, ServerResponse{}, err
	}

	questions := make([]map[string]any, 0, len(params.Questions))
	for _, question := range params.Questions {
		item := map[string]any{
			"id":       question.ID,
			"header":   question.Header,
			"question": question.Question,
		}
		if len(question.Options) > 0 {
			options := make([]map[string]any, 0, len(question.Options))
			for _, option := range question.Options {
				options = append(options, map[string]any{
					"label":       option.Label,
					"description": option.Description,
				})
			}
			item["options"] = options
		}
		questions = append(questions, item)
	}

	tool := map[string]any{
		"kind":     "tool",
		"toolKind": "ask_user_question",
		"toolName": "AskUserQuestion",
		"toolId":   params.ItemID,
		"input": map[string]any{
			"questions": questions,
		},
		"rawInput": map[string]any{
			"questions": params.Questions,
		},
	}
	events := []HarnessEvent{{
		Type: "transcript",
		Entry: transcript.New(transcript.KindToolCall, map[string]any{
			"tool": tool,
		}),
	}}

	result := map[string]any{}
	if handlers.OnToolRequest != nil {
		var err error
		result, err = handlers.OnToolRequest(ToolRequest{Tool: tool})
		if err != nil {
			return events, ServerResponse{}, err
		}
	}
	return events, ServerResponse{
		ID: request.ID,
		Result: map[string]any{
			"answers": normalizeUserInputAnswers(result, params.Questions),
		},
	}, nil
}

func handleApprovalRequest(request ServerRequest, handlers RequestHandlers, kind string) ([]HarnessEvent, ServerResponse, error) {
	decision := "decline"
	if handlers.OnApprovalRequest != nil {
		var err error
		decision, err = handlers.OnApprovalRequest(ApprovalRequest{
			RequestID: request.ID,
			Kind:      kind,
			Params:    request.Params,
		})
		if err != nil {
			return nil, ServerResponse{}, err
		}
	}
	return nil, ServerResponse{
		ID: request.ID,
		Result: map[string]any{
			"decision": decision,
		},
	}, nil
}

func normalizeUserInputAnswers(result map[string]any, questions []struct {
	ID       string `json:"id"`
	Header   string `json:"header"`
	Question string `json:"question"`
	Options  []struct {
		Label       string `json:"label"`
		Description string `json:"description,omitempty"`
	} `json:"options"`
}) map[string]any {
	answerRoot, _ := result["answers"].(map[string]any)
	answers := map[string]any{}
	for _, question := range questions {
		values := answerValues(answerRoot[question.ID])
		if len(values) == 0 {
			values = answerValues(answerRoot[question.Question])
		}
		answers[question.ID] = map[string]any{"answers": values}
	}
	return answers
}

func answerValues(value any) []string {
	switch typed := value.(type) {
	case string:
		if typed == "" {
			return nil
		}
		return []string{typed}
	case []string:
		return typed
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				values = append(values, text)
			}
		}
		return values
	default:
		return nil
	}
}
