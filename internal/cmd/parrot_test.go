package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunParrot_PrintsBannerWithContactPointer(t *testing.T) {
	buffer := &bytes.Buffer{}
	parrotCmd.SetOut(buffer)

	if err := runParrot(parrotCmd, nil); err != nil {
		t.Fatalf("runParrot: %v", err)
	}

	printed := buffer.String()
	if !strings.Contains(printed, "AraraHQ") {
		t.Errorf("banner missing brand name: %q", printed)
	}
	if !strings.Contains(printed, "oi@ararahq.com") {
		t.Errorf("banner missing contact pointer")
	}
}

func TestParrotCmd_IsHiddenFromHelp(t *testing.T) {
	if !parrotCmd.Hidden {
		t.Error("parrot must stay hidden -- easter eggs do not appear in help")
	}
}
