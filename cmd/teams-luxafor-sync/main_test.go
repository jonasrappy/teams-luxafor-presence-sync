package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestNoDeviceBackoffAfterFiveConsecutiveErrors(t *testing.T) {
	a := &app{}
	now := time.Unix(1700000000, 0)

	for i := 0; i < noDeviceErrorThreshold-1; i++ {
		a.recordLuxaforWriteResult(errNoLuxaforDevice, now)
	}
	if !a.noDeviceBackoffUntil.IsZero() {
		t.Fatalf("expected no backoff before threshold, got %v", a.noDeviceBackoffUntil)
	}

	a.recordLuxaforWriteResult(errNoLuxaforDevice, now)
	wantBackoffUntil := now.Add(noDeviceRetryInterval)
	if !a.noDeviceBackoffUntil.Equal(wantBackoffUntil) {
		t.Fatalf("expected backoff until %v, got %v", wantBackoffUntil, a.noDeviceBackoffUntil)
	}
	if a.canAttemptLuxaforWrite(now.Add(30 * time.Second)) {
		t.Fatalf("expected write to be blocked during backoff window")
	}
	if !a.canAttemptLuxaforWrite(now.Add(61 * time.Second)) {
		t.Fatalf("expected write to be allowed after backoff window")
	}
}

func TestNoDeviceBackoffResetsOnSuccess(t *testing.T) {
	a := &app{}
	now := time.Unix(1700000000, 0)

	for i := 0; i < noDeviceErrorThreshold; i++ {
		a.recordLuxaforWriteResult(errNoLuxaforDevice, now)
	}
	if a.noDeviceBackoffUntil.IsZero() {
		t.Fatalf("expected backoff to be active")
	}

	a.recordLuxaforWriteResult(nil, now.Add(2*time.Second))
	if a.noDeviceErrorCount != 0 {
		t.Fatalf("expected error count reset, got %d", a.noDeviceErrorCount)
	}
	if !a.noDeviceBackoffUntil.IsZero() {
		t.Fatalf("expected backoff cleared, got %v", a.noDeviceBackoffUntil)
	}
}

func TestNoDeviceCounterResetsOnOtherError(t *testing.T) {
	a := &app{}
	now := time.Unix(1700000000, 0)

	for i := 0; i < noDeviceErrorThreshold-1; i++ {
		a.recordLuxaforWriteResult(errNoLuxaforDevice, now)
	}
	a.recordLuxaforWriteResult(errors.New("write failed"), now)

	if a.noDeviceErrorCount != 0 {
		t.Fatalf("expected no-device error count reset, got %d", a.noDeviceErrorCount)
	}
	if !a.noDeviceBackoffUntil.IsZero() {
		t.Fatalf("expected no backoff after other error, got %v", a.noDeviceBackoffUntil)
	}
}
