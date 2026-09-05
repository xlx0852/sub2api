package handler

import "testing"

func TestParseOpenAIResponsesIngress(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		model    string
		stream   bool
		previous string
		ok       bool
	}{
		{"valid", `{"model":"gpt-5.1","stream":true,"previous_response_id":" resp_1 "}`, "gpt-5.1", true, "resp_1", true},
		{"missing model", `{"stream":true}`, "", false, "", false},
		{"invalid stream", `{"model":"gpt-5.1","stream":"yes"}`, "", false, "", false},
		{"invalid json", `{`, "", false, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, ok := parseOpenAIResponsesIngress([]byte(tt.body))
			if ok != tt.ok || got.model != tt.model || got.stream != tt.stream || got.previousResponseID != tt.previous {
				t.Fatalf("got %#v, ok=%v", got, ok)
			}
		})
	}
}
