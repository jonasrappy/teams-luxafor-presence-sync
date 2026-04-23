package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractAvailabilityFromBytes_LastMatchWins(t *testing.T) {
	input := []byte(`foo availability: Busy
bar "availability":"Available"
baz availability = DoNotDisturb`)

	got := extractAvailabilityFromBytes(input)
	if got != "DoNotDisturb" {
		t.Fatalf("expected DoNotDisturb, got %q", got)
	}
}

func TestExtractAvailabilityFromBytes_CaseInsensitiveKey(t *testing.T) {
	input := []byte(`{"Availability":"InAMeeting"}`)

	got := extractAvailabilityFromBytes(input)
	if got != "InAMeeting" {
		t.Fatalf("expected InAMeeting, got %q", got)
	}
}

func TestMapToColor(t *testing.T) {
	if got := mapToColor("DoNotDisturb"); got != "red" {
		t.Fatalf("expected red for DoNotDisturb, got %q", got)
	}
	if got := mapToColor("BeRightBack"); got != "yellow" {
		t.Fatalf("expected yellow for BeRightBack, got %q", got)
	}
	if got := mapToColor("Away"); got != "yellow" {
		t.Fatalf("expected yellow for Away, got %q", got)
	}
	if got := mapToColor("Available"); got != "green" {
		t.Fatalf("expected green for Available, got %q", got)
	}
}

func TestExtractAvailabilityFromFile_TailOnly(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "MSTeams_test.log")
	prefix := strings.Repeat("x", 8192)
	content := prefix + "\navailability: Busy\n" + strings.Repeat("y", 4096) + "\navailability: Available\n"
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	a := &app{tailBytes: 2048}
	got, err := a.extractAvailabilityFromFile(filePath)
	if err != nil {
		t.Fatalf("extract availability: %v", err)
	}
	if got != "Available" {
		t.Fatalf("expected Available from tail section, got %q", got)
	}
}
