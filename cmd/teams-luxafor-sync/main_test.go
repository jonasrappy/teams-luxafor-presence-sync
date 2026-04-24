package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	hid "github.com/sstallion/go-hid"
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

func TestColorRGB(t *testing.T) {
	tests := []struct {
		name string
		r    byte
		g    byte
		b    byte
	}{
		{name: "red", r: 255, g: 0, b: 0},
		{name: "green", r: 0, g: 255, b: 0},
		{name: "yellow", r: 255, g: 180, b: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, g, b, err := colorRGB(tt.name)
			if err != nil {
				t.Fatalf("colorRGB(%q): %v", tt.name, err)
			}
			if r != tt.r || g != tt.g || b != tt.b {
				t.Fatalf("expected RGB(%d,%d,%d), got RGB(%d,%d,%d)", tt.r, tt.g, tt.b, r, g, b)
			}
		})
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

func TestHIDSessionResetsAfterNoDeviceError(t *testing.T) {
	api := &fakeHIDAPI{}
	session := newHIDSession(api)
	report := []byte{1, 2, 3}

	err := session.writeReport(vendorID, productID, report)
	if !errors.Is(err, errNoLuxaforDevice) {
		t.Fatalf("expected no-device error, got %v", err)
	}
	if api.initCalls != 1 {
		t.Fatalf("expected one HID init, got %d", api.initCalls)
	}
	if api.exitCalls != 1 {
		t.Fatalf("expected HID reset after no-device error, got %d exits", api.exitCalls)
	}
	if session.initialized {
		t.Fatalf("expected session to be marked uninitialized after reset")
	}

	err = session.writeReport(vendorID, productID, report)
	if !errors.Is(err, errNoLuxaforDevice) {
		t.Fatalf("expected second no-device error, got %v", err)
	}
	if api.initCalls != 2 {
		t.Fatalf("expected fresh HID init on retry, got %d", api.initCalls)
	}
}

func TestHIDSessionReinitializesAndWritesAfterDeviceReturns(t *testing.T) {
	report := []byte{1, 255, 0, 255, 0, 0, 0, 0}
	api := &fakeHIDAPI{
		enumerations: []fakeEnumeration{
			{},
			{paths: []string{"returned-device"}},
		},
	}
	session := newHIDSession(api)

	err := session.writeReport(vendorID, productID, report)
	if !errors.Is(err, errNoLuxaforDevice) {
		t.Fatalf("expected no-device error, got %v", err)
	}

	err = session.writeReport(vendorID, productID, report)
	if err != nil {
		t.Fatalf("expected write to recover after device returns, got %v", err)
	}
	if api.initCalls != 2 {
		t.Fatalf("expected HID to be reinitialized before recovery write, got %d inits", api.initCalls)
	}
	if api.exitCalls != 1 {
		t.Fatalf("expected one HID reset before recovery, got %d exits", api.exitCalls)
	}
	if len(api.openPaths) != 1 || api.openPaths[0] != "returned-device" {
		t.Fatalf("expected returned device to be opened, got %v", api.openPaths)
	}
	if len(api.writes) != 1 || !bytes.Equal(api.writes[0], report) {
		t.Fatalf("expected report write %v, got %v", report, api.writes)
	}
	if !session.initialized {
		t.Fatalf("expected successful session to remain initialized")
	}
}

func TestHIDSessionResetsAndBackoffCanClassifyOpenError(t *testing.T) {
	api := &fakeHIDAPI{
		enumerations: []fakeEnumeration{{paths: []string{"stale-device"}}},
		openErrs:     []error{errors.New("stale path")},
	}
	session := newHIDSession(api)

	err := session.writeReport(vendorID, productID, []byte{1, 2, 3})
	if !errors.Is(err, errNoLuxaforDevice) {
		t.Fatalf("expected open failure to wrap no-device error, got %v", err)
	}
	if api.exitCalls != 1 {
		t.Fatalf("expected HID reset after open error, got %d exits", api.exitCalls)
	}

	a := &app{}
	now := time.Unix(1700000000, 0)
	for i := 0; i < noDeviceErrorThreshold; i++ {
		a.recordLuxaforWriteResult(err, now)
	}
	if a.noDeviceBackoffUntil.IsZero() {
		t.Fatalf("expected wrapped open failure to trigger no-device backoff")
	}
}

type fakeEnumeration struct {
	paths []string
	err   error
}

type fakeHIDAPI struct {
	initCalls    int
	exitCalls    int
	closeCalls   int
	enumerations []fakeEnumeration
	openErrs     []error
	openPaths    []string
	writes       [][]byte
}

func (f *fakeHIDAPI) Init() error {
	f.initCalls++
	return nil
}

func (f *fakeHIDAPI) Exit() error {
	f.exitCalls++
	return nil
}

func (f *fakeHIDAPI) Enumerate(vid, pid uint16, enumFn func(*hid.DeviceInfo) error) error {
	enumeration := fakeEnumeration{}
	if len(f.enumerations) > 0 {
		enumeration = f.enumerations[0]
		f.enumerations = f.enumerations[1:]
	}

	for _, path := range enumeration.paths {
		if err := enumFn(&hid.DeviceInfo{
			Path:      path,
			VendorID:  vid,
			ProductID: pid,
		}); err != nil {
			return err
		}
	}
	return enumeration.err
}

func (f *fakeHIDAPI) OpenPath(path string) (hidDevice, error) {
	f.openPaths = append(f.openPaths, path)
	if len(f.openErrs) > 0 {
		err := f.openErrs[0]
		f.openErrs = f.openErrs[1:]
		if err != nil {
			return nil, err
		}
	}
	return &fakeHIDDevice{api: f}, nil
}

type fakeHIDDevice struct {
	api *fakeHIDAPI
}

func (d *fakeHIDDevice) Write(p []byte) (int, error) {
	d.api.writes = append(d.api.writes, append([]byte(nil), p...))
	return len(p), nil
}

func (d *fakeHIDDevice) Close() error {
	d.api.closeCalls++
	return nil
}
