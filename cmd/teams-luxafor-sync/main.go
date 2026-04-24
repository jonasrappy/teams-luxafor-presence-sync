package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	hid "github.com/sstallion/go-hid"
)

const (
	vendorID               = 1240
	productID              = 62322
	noDeviceErrorThreshold = 5
	noDeviceRetryInterval  = 60 * time.Second
)

var version = "dev"

var busyStatuses = map[string]struct{}{
	"busy":              {},
	"donotdisturb":      {},
	"inacall":           {},
	"inaconferencecall": {},
	"inameeting":        {},
	"presenting":        {},
	"focusing":          {},
}

var awayStatuses = map[string]struct{}{
	"berightback": {},
	"away":        {},
	"appearaway":  {},
}

var availabilityRe = regexp.MustCompile(`(?i)availability["']?\s*[:=]\s*["']?([A-Za-z]+)`)
var hidInitOnce sync.Once
var hidInitErr error
var errEnumerationDone = errors.New("enumeration done")
var errNoLuxaforDevice = errors.New("no Luxafor device found")

type app struct {
	pollInterval         time.Duration
	tailBytes            int64
	fallbackLogScanCount int
	reapplyInterval      time.Duration
	manualTeamsLogDir    string
	cachedLogDir         string
	lastState            string
	lastColor            string
	lastLogFile          string
	lastNoLogMessageAt   time.Time
	lastApplyAt          time.Time
	noDeviceErrorCount   int
	noDeviceBackoffUntil time.Time
}

type logFile struct {
	path    string
	modTime time.Time
}

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	showVersion := flag.Bool("version", false, "print version")
	runOnce := flag.Bool("once", false, "run one sync tick and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	a := &app{
		pollInterval:         durationFromEnv("POLL_MS", 300*time.Millisecond),
		tailBytes:            int64(intFromEnv("TAIL_BYTES", 256*1024)),
		fallbackLogScanCount: intFromEnv("FALLBACK_LOG_SCAN_COUNT", 5),
		reapplyInterval:      durationFromEnv("REAPPLY_MS", 15000*time.Millisecond),
		manualTeamsLogDir:    strings.TrimSpace(os.Getenv("TEAMS_LOG_DIR")),
	}

	log.Printf("Teams -> Luxafor sync started")
	a.tick()
	if *runOnce {
		return
	}
	ticker := time.NewTicker(a.pollInterval)
	defer ticker.Stop()

	for range ticker.C {
		a.tick()
	}
}

func durationFromEnv(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return time.Duration(n) * time.Millisecond
}

func intFromEnv(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func (a *app) tick() {
	logDir := a.resolveLogDir()
	latestFile := a.getLatestTeamsLogFile()
	if logDir == "" || latestFile == "" {
		if time.Since(a.lastNoLogMessageAt) > time.Minute {
			log.Printf("No Teams logs found. Set TEAMS_LOG_DIR if needed.")
			a.lastNoLogMessageAt = time.Now()
		}
		return
	}

	if a.lastLogFile != latestFile {
		a.lastLogFile = latestFile
		log.Printf("Using log directory: %s", logDir)
		log.Printf("Following %s", latestFile)
	}

	availability, err := a.extractAvailabilityFromFile(latestFile)
	if err != nil {
		log.Printf("Sync error: %v", err)
		return
	}
	if availability == "" {
		availability = a.findMostRecentAvailability()
	}
	if availability == "" {
		return
	}

	color := mapToColor(availability)
	needsReapply := time.Since(a.lastApplyAt) >= a.reapplyInterval
	if availability != a.lastState || color != a.lastColor || needsReapply {
		now := time.Now()
		if !a.canAttemptLuxaforWrite(now) {
			return
		}

		if err := setLuxaforColor(color); err != nil {
			a.recordLuxaforWriteResult(err, now)
			log.Printf("Luxafor error: %v", err)
			return
		}

		a.recordLuxaforWriteResult(nil, now)
		a.lastState = availability
		a.lastColor = color
		a.lastApplyAt = now
		log.Printf("Teams=%s -> Luxafor=%s", availability, color)
	}
}

func (a *app) canAttemptLuxaforWrite(now time.Time) bool {
	return !a.noDeviceBackoffUntil.After(now)
}

func (a *app) recordLuxaforWriteResult(err error, now time.Time) {
	if err == nil {
		a.noDeviceErrorCount = 0
		a.noDeviceBackoffUntil = time.Time{}
		return
	}

	if !errors.Is(err, errNoLuxaforDevice) {
		a.noDeviceErrorCount = 0
		a.noDeviceBackoffUntil = time.Time{}
		return
	}

	a.noDeviceErrorCount++
	if a.noDeviceErrorCount < noDeviceErrorThreshold {
		return
	}

	a.noDeviceBackoffUntil = now.Add(noDeviceRetryInterval)
}

func (a *app) resolveLogDir() string {
	if a.cachedLogDir != "" && hasTeamsLog(a.cachedLogDir) {
		return a.cachedLogDir
	}

	for _, candidate := range a.listLogCandidates() {
		if hasTeamsLog(candidate) {
			a.cachedLogDir = candidate
			return candidate
		}
	}
	return ""
}

func (a *app) listLogCandidates() []string {
	home, _ := os.UserHomeDir()
	candidates := []string{}

	if a.manualTeamsLogDir != "" {
		candidates = append(candidates, a.manualTeamsLogDir)
	}

	candidates = append(candidates,
		filepath.Join(home, "Library/Group Containers/UBF8T346G9.com.microsoft.teams/Library/Application Support/Logs"),
		filepath.Join(home, "Library/Application Support/Microsoft/Teams/logs"),
	)

	groupRoot := filepath.Join(home, "Library/Group Containers")
	entries, err := os.ReadDir(groupRoot)
	if err == nil {
		for _, e := range entries {
			name := strings.ToLower(e.Name())
			if strings.Contains(name, "microsoft.teams") {
				candidates = append(candidates, filepath.Join(groupRoot, e.Name(), "Library/Application Support/Logs"))
			}
		}
	}

	seen := map[string]struct{}{}
	uniq := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		uniq = append(uniq, c)
	}
	return uniq
}

func hasTeamsLog(dirPath string) bool {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return false
	}
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, "MSTeams_") && strings.HasSuffix(n, ".log") {
			return true
		}
	}
	return false
}

func (a *app) getTeamsLogFilesSorted() []logFile {
	logDir := a.resolveLogDir()
	if logDir == "" {
		return nil
	}
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return nil
	}

	files := []logFile{}
	for _, e := range entries {
		n := e.Name()
		if !strings.HasPrefix(n, "MSTeams_") || !strings.HasSuffix(n, ".log") {
			continue
		}
		full := filepath.Join(logDir, n)
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, logFile{path: full, modTime: info.ModTime()})
	}

	sort.Slice(files, func(i, j int) bool { return files[i].modTime.After(files[j].modTime) })
	return files
}

func (a *app) getLatestTeamsLogFile() string {
	files := a.getTeamsLogFilesSorted()
	if len(files) == 0 {
		return ""
	}
	return files[0].path
}

func (a *app) extractAvailabilityFromFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", err
	}

	size := info.Size()
	if size <= 0 {
		return "", nil
	}
	tail := a.tailBytes
	if tail <= 0 {
		tail = size
	}
	start := int64(0)
	if size > tail {
		start = size - tail
	}

	length := size - start
	data := make([]byte, int(length))
	if _, err := file.ReadAt(data, start); err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}

	return extractAvailabilityFromBytes(data), nil
}

func extractAvailabilityFromBytes(data []byte) string {
	matches := availabilityRe.FindAllSubmatch(data, -1)
	if len(matches) == 0 {
		return ""
	}
	last := matches[len(matches)-1]
	if len(last) < 2 {
		return ""
	}
	return string(last[1])
}

func (a *app) findMostRecentAvailability() string {
	files := a.getTeamsLogFilesSorted()
	if len(files) == 0 {
		return ""
	}
	max := a.fallbackLogScanCount
	if max > len(files) {
		max = len(files)
	}
	for i := 0; i < max; i++ {
		availability, err := a.extractAvailabilityFromFile(files[i].path)
		if err == nil && availability != "" {
			return availability
		}
	}
	return ""
}

func mapToColor(status string) string {
	normalized := strings.ToLower(strings.TrimSpace(status))
	_, busy := busyStatuses[normalized]
	if busy {
		return "red"
	}
	_, away := awayStatuses[normalized]
	if away {
		return "yellow"
	}
	return "green"
}

func setLuxaforColor(color string) error {
	r, g, b, err := colorRGB(color)
	if err != nil {
		return err
	}

	report := []byte{1, 255, r, g, b, 0, 0, 0}

	hidInitOnce.Do(func() {
		hidInitErr = hid.Init()
	})
	if hidInitErr != nil {
		return hidInitErr
	}

	found := false
	writeErr := errNoLuxaforDevice

	err = hid.Enumerate(vendorID, productID, func(info *hid.DeviceInfo) error {
		found = true
		dev, openErr := hid.OpenPath(info.Path)
		if openErr != nil {
			writeErr = fmt.Errorf("cannot open device with vendor id 0x4d8 and product id 0xf372: %v", openErr)
			return nil
		}
		defer dev.Close()

		if _, openErr = dev.Write(report); openErr != nil {
			writeErr = openErr
			return nil
		}

		writeErr = nil
		return errEnumerationDone
	})
	if err != nil && !errors.Is(err, errEnumerationDone) {
		return err
	}
	if !found {
		return errNoLuxaforDevice
	}
	return writeErr
}

func colorRGB(name string) (byte, byte, byte, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "red":
		return 255, 0, 0, nil
	case "green":
		return 0, 255, 0, nil
	case "yellow":
		return 255, 255, 0, nil
	default:
		return 0, 0, 0, fmt.Errorf("unsupported color: %s", name)
	}
}
