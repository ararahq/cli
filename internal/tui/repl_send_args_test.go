package tui

import (
	"reflect"
	"testing"
)

func TestParseSendArgs(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		wantPhone        string
		wantTemplateName string
		wantVariables    []string
		wantFreeformText string
	}{
		{
			name:             "positional + short flag",
			input:            "+5511999999999 -t welcome",
			wantPhone:        "+5511999999999",
			wantTemplateName: "welcome",
		},
		{
			name:             "positional + long flag",
			input:            "+5511999999999 --template welcome",
			wantPhone:        "+5511999999999",
			wantTemplateName: "welcome",
		},
		{
			name:             "CLI-shell style: --to + --template",
			input:            "--to +5511999999999 --template welcome",
			wantPhone:        "+5511999999999",
			wantTemplateName: "welcome",
		},
		{
			name:             "with vars",
			input:            "+5511 -t hi -v João,Doe",
			wantPhone:        "+5511",
			wantTemplateName: "hi",
			wantVariables:    []string{"João", "Doe"},
		},
		{
			name:             "freeform when no template flag",
			input:            "+5511999999999 Hello there",
			wantPhone:        "+5511999999999",
			wantFreeformText: "Hello there",
		},
		{
			name:  "unknown flag returns empty (REPL shows usage)",
			input: "+5511 --foo bar",
		},
		{
			name: "empty input returns zero values",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			phone, templateName, variables, freeformText := parseSendArgs(testCase.input)
			if phone != testCase.wantPhone {
				t.Errorf("phone = %q, want %q", phone, testCase.wantPhone)
			}
			if templateName != testCase.wantTemplateName {
				t.Errorf("templateName = %q, want %q", templateName, testCase.wantTemplateName)
			}
			if !reflect.DeepEqual(variables, testCase.wantVariables) && (len(variables) != 0 || len(testCase.wantVariables) != 0) {
				t.Errorf("variables = %v, want %v", variables, testCase.wantVariables)
			}
			if freeformText != testCase.wantFreeformText {
				t.Errorf("freeformText = %q, want %q", freeformText, testCase.wantFreeformText)
			}
		})
	}
}
