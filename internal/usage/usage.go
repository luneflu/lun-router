// Package usage records one append-only JSONL event per completion request and
// aggregates them for the dashboard. Events never contain prompt or completion
// text, only counters.
package usage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Tokens is the mutable accumulator the response translators fill in. It is only
// touched from the request goroutine.
type Tokens struct {
	In        int
	Out       int
	Cached    int
	Reasoning int
	Src       string // "upstream" when the provider reported them, else empty
}

// Set records provider-reported usage.
func (t *Tokens) Set(in, out, cached, reasoning int) {
	if t == nil {
		return
	}
	t.In, t.Out, t.Cached, t.Reasoning = in, out, cached, reasoning
	t.Src = "upstream"
}

// Event is one completed request.
type Event struct {
	TS        time.Time `json:"ts"`
	KeyID     string    `json:"key_id,omitempty"`
	KeyName   string    `json:"key_name,omitempty"`
	Provider  string    `json:"prov"`
	Account   string    `json:"acct,omitempty"`
	Model     string    `json:"model"`
	ReqModel  string    `json:"req_model,omitempty"`
	Endpoint  string    `json:"ep,omitempty"`
	Stream    bool      `json:"stream,omitempty"`
	Status    int       `json:"status"`
	Ms        int64     `json:"ms"`
	UpMs      int64     `json:"up_ms,omitempty"`
	In        int       `json:"in"`
	Out       int       `json:"out"`
	Cached    int       `json:"cached,omitempty"`
	Reasoning int       `json:"reasoning,omitempty"`
	TokSrc    string    `json:"tok_src,omitempty"`
	Err       string    `json:"err,omitempty"`
	Saved     int       `json:"saved,omitempty"`
	Filters   []string  `json:"filters,omitempty"`
	Agent     string    `json:"agent,omitempty"`
	Sess      string    `json:"sess,omitempty"`
}

type Service struct {
	mu    sync.Mutex
	dir   string
	file  *os.File
	day   string
	cache *cachedSummary
}

const cacheTTL = 15 * time.Second

type cachedSummary struct {
	key string
	at  time.Time
	sum Summary
}

func NewService(dataDir string) (*Service, error) {
	dir := filepath.Join(dataDir, "usage")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create usage dir: %w", err)
	}
	return &Service{dir: dir}, nil
}

// Record appends one event. Failures are logged, never returned: monitoring must
// not break a request.
func (s *Service) Record(e Event) {
	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	}
	line, err := json.Marshal(e)
	if err != nil {
		log.Printf("[usage] marshal: %v", err)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := s.fileFor(e.TS)
	if err != nil {
		log.Printf("[usage] %v", err)
		return
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		log.Printf("[usage] write: %v", err)
	}
}

// fileFor returns the day's append handle, rotating on date change.
func (s *Service) fileFor(ts time.Time) (*os.File, error) {
	day := ts.UTC().Format("2006-01-02")
	if s.file != nil && s.day == day {
		return s.file, nil
	}
	if s.file != nil {
		_ = s.file.Close()
		s.file = nil
	}
	f, err := os.OpenFile(filepath.Join(s.dir, day+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open usage file: %w", err)
	}
	s.file, s.day = f, day
	return f, nil
}

// Close releases the current file handle.
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return nil
	}
	err := s.file.Close()
	s.file, s.day = nil, ""
	return err
}

type Total struct {
	Requests   int   `json:"requests"`
	Errors     int   `json:"errors"`
	In         int   `json:"in"`
	Out        int   `json:"out"`
	Cached     int   `json:"cached"`
	Reasoning  int   `json:"reasoning"`
	SavedBytes int   `json:"saved_bytes"`
	AvgMs      int64 `json:"avg_ms"`
}

type DayRow struct {
	Date     string `json:"date"`
	Requests int    `json:"requests"`
	Errors   int    `json:"errors"`
	In       int    `json:"in"`
	Out      int    `json:"out"`
}

type KeyRow struct {
	KeyID    string `json:"key_id"`
	KeyName  string `json:"key_name"`
	Requests int    `json:"requests"`
	Errors   int    `json:"errors"`
	In       int    `json:"in"`
	Out      int    `json:"out"`
	LastTS   string `json:"last_ts"`
}

type ProviderRow struct {
	Provider string `json:"prov"`
	Requests int    `json:"requests"`
	Errors   int    `json:"errors"`
	In       int    `json:"in"`
	Out      int    `json:"out"`
}

type HourRow struct {
	Hour     int `json:"hour"`
	Requests int `json:"requests"`
	Errors   int `json:"errors"`
	In       int `json:"in"`
	Out      int `json:"out"`
}

// Filter narrows the events considered by Summary. Empty fields match everything.
type Filter struct {
	Provider string
	KeyID    string
	Model    string
}

func (f Filter) empty() bool {
	return f.Provider == "" && f.KeyID == "" && f.Model == ""
}

func (f Filter) matches(e *Event) bool {
	if f.Provider != "" && e.Provider != f.Provider {
		return false
	}
	if f.KeyID != "" && e.KeyID != f.KeyID {
		return false
	}
	if f.Model != "" && e.Model != f.Model {
		return false
	}
	return true
}

type ModelRow struct {
	Provider string `json:"prov"`
	Model    string `json:"model"`
	Requests int    `json:"requests"`
	Errors   int    `json:"errors"`
	In       int    `json:"in"`
	Out      int    `json:"out"`
}

type Summary struct {
	Days       int           `json:"days"`
	From       string        `json:"from"`
	To         string        `json:"to"`
	Filters    FilterView    `json:"filters"`
	Totals     Total         `json:"totals"`
	ByDay      []DayRow      `json:"by_day"`
	ByHour     []HourRow     `json:"by_hour,omitempty"`
	ByProvider []ProviderRow `json:"by_provider"`
	ByKey      []KeyRow      `json:"by_key"`
	ByModel    []ModelRow    `json:"by_model"`
	Recent     []Event       `json:"recent"`
}

// FilterView echoes the applied filter back to the client.
type FilterView struct {
	Provider string `json:"prov,omitempty"`
	KeyID    string `json:"key_id,omitempty"`
	Model    string `json:"model,omitempty"`
}

const recentLimit = 50

// Summary aggregates the last N days of events (including today), optionally
// narrowed by f. A single-day window also gets hourly buckets.
func (s *Service) Summary(days int, f Filter) (Summary, error) {
	if days < 1 {
		days = 1
	}
	if days > 365 {
		days = 365
	}

	cacheKey := fmt.Sprintf("%d|%s|%s|%s", days, f.Provider, f.KeyID, f.Model)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cache != nil && s.cache.key == cacheKey && time.Since(s.cache.at) < cacheTTL {
		return s.cache.sum, nil
	}

	now := time.Now().UTC()
	from := now.AddDate(0, 0, -(days - 1)).Truncate(24 * time.Hour)

	sum := Summary{
		Days:       days,
		From:       from.Format("2006-01-02"),
		To:         now.Format("2006-01-02"),
		Filters:    FilterView{Provider: f.Provider, KeyID: f.KeyID, Model: f.Model},
		ByKey:      []KeyRow{},
		ByModel:    []ModelRow{},
		ByProvider: []ProviderRow{},
		Recent:     []Event{},
	}
	if days == 1 {
		sum.ByHour = make([]HourRow, 24)
		for i := range sum.ByHour {
			sum.ByHour[i].Hour = i
		}
	}
	dayIndex := map[string]int{}
	keyIndex := map[string]int{}
	modelIndex := map[string]int{}
	provIndex := map[string]int{}
	var msTotal int64

	for d := 0; d < days; d++ {
		date := from.AddDate(0, 0, d).Format("2006-01-02")
		dayIndex[date] = len(sum.ByDay)
		sum.ByDay = append(sum.ByDay, DayRow{Date: date})
	}

	files, err := filepath.Glob(filepath.Join(s.dir, "*.jsonl"))
	if err != nil {
		return sum, fmt.Errorf("list usage files: %w", err)
	}
	sort.Strings(files)

	for _, path := range files {
		date := strings.TrimSuffix(filepath.Base(path), ".jsonl")
		dayPos, ok := dayIndex[date]
		if !ok {
			continue
		}
		if err := s.scanFile(path, dayPos, f, &sum, keyIndex, modelIndex, provIndex, &msTotal); err != nil {
			log.Printf("[usage] scan %s: %v", path, err)
		}
	}

	if sum.Totals.Requests > 0 {
		sum.Totals.AvgMs = msTotal / int64(sum.Totals.Requests)
	}
	// newest first for the dashboard
	for i, j := 0, len(sum.Recent)-1; i < j; i, j = i+1, j-1 {
		sum.Recent[i], sum.Recent[j] = sum.Recent[j], sum.Recent[i]
	}
	sort.SliceStable(sum.ByKey, func(i, j int) bool {
		if sum.ByKey[i].Requests != sum.ByKey[j].Requests {
			return sum.ByKey[i].Requests > sum.ByKey[j].Requests
		}
		return sum.ByKey[i].KeyName < sum.ByKey[j].KeyName
	})
	sort.SliceStable(sum.ByProvider, func(i, j int) bool {
		if sum.ByProvider[i].Requests != sum.ByProvider[j].Requests {
			return sum.ByProvider[i].Requests > sum.ByProvider[j].Requests
		}
		return sum.ByProvider[i].Provider < sum.ByProvider[j].Provider
	})
	sort.SliceStable(sum.ByModel, func(i, j int) bool {
		if sum.ByModel[i].Requests != sum.ByModel[j].Requests {
			return sum.ByModel[i].Requests > sum.ByModel[j].Requests
		}
		if sum.ByModel[i].Provider != sum.ByModel[j].Provider {
			return sum.ByModel[i].Provider < sum.ByModel[j].Provider
		}
		return sum.ByModel[i].Model < sum.ByModel[j].Model
	})

	s.cache = &cachedSummary{key: cacheKey, at: time.Now(), sum: sum}
	return sum, nil
}

func (s *Service) scanFile(path string, dayPos int, f Filter, sum *Summary, keyIndex, modelIndex, provIndex map[string]int, msTotal *int64) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			continue // skip corrupt lines rather than losing the whole day
		}
		if !f.matches(&e) {
			continue
		}

		sum.Totals.Requests++
		sum.Totals.In += e.In
		sum.Totals.Out += e.Out
		sum.Totals.Cached += e.Cached
		sum.Totals.Reasoning += e.Reasoning
		sum.Totals.SavedBytes += e.Saved
		*msTotal += e.Ms
		if e.Status >= 400 {
			sum.Totals.Errors++
			sum.ByDay[dayPos].Errors++
		}
		sum.ByDay[dayPos].Requests++
		sum.ByDay[dayPos].In += e.In
		sum.ByDay[dayPos].Out += e.Out
		if sum.ByHour != nil {
			h := &sum.ByHour[e.TS.UTC().Hour()]
			h.Requests++
			h.In += e.In
			h.Out += e.Out
			if e.Status >= 400 {
				h.Errors++
			}
		}

		kid := e.KeyID
		if kid == "" {
			kid = "unknown"
		}
		ki, ok := keyIndex[kid]
		if !ok {
			ki = len(sum.ByKey)
			keyIndex[kid] = ki
			sum.ByKey = append(sum.ByKey, KeyRow{KeyID: e.KeyID, KeyName: e.KeyName})
		}
		sum.ByKey[ki].Requests++
		sum.ByKey[ki].In += e.In
		sum.ByKey[ki].Out += e.Out
		if e.Status >= 400 {
			sum.ByKey[ki].Errors++
		}
		ts := e.TS.UTC().Format(time.RFC3339)
		if ts > sum.ByKey[ki].LastTS {
			sum.ByKey[ki].LastTS = ts
		}

		pi, ok := provIndex[e.Provider]
		if !ok {
			pi = len(sum.ByProvider)
			provIndex[e.Provider] = pi
			sum.ByProvider = append(sum.ByProvider, ProviderRow{Provider: e.Provider})
		}
		sum.ByProvider[pi].Requests++
		sum.ByProvider[pi].In += e.In
		sum.ByProvider[pi].Out += e.Out
		if e.Status >= 400 {
			sum.ByProvider[pi].Errors++
		}

		mid := e.Provider + "\x00" + e.Model
		mi, ok := modelIndex[mid]
		if !ok {
			mi = len(sum.ByModel)
			modelIndex[mid] = mi
			sum.ByModel = append(sum.ByModel, ModelRow{Provider: e.Provider, Model: e.Model})
		}
		sum.ByModel[mi].Requests++
		sum.ByModel[mi].In += e.In
		sum.ByModel[mi].Out += e.Out
		if e.Status >= 400 {
			sum.ByModel[mi].Errors++
		}

		sum.Recent = append(sum.Recent, e)
		if len(sum.Recent) > recentLimit {
			sum.Recent = sum.Recent[1:]
		}
	}

	return scanner.Err()
}
