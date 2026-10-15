package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecordAndSummary(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defer svc.Close()

	now := time.Now().UTC()
	svc.Record(Event{TS: now, KeyID: "k1", KeyName: "laptop", Provider: "opencode", Model: "big-pickle", Status: 200, Ms: 100, In: 10, Out: 5, Saved: 7, Filters: []string{"json"}, TokSrc: "upstream"})
	svc.Record(Event{TS: now, KeyID: "k1", KeyName: "laptop", Provider: "opencode", Model: "big-pickle", Status: 502, Ms: 200, Err: "upstream_error"})
	svc.Record(Event{TS: now, KeyID: "k2", KeyName: "ci", Provider: "antigravity", Model: "gemini-3.8-flash", Status: 200, Ms: 300, In: 100, Out: 50, Cached: 20, Reasoning: 5})

	sum, err := svc.Summary(7, Filter{})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}

	if sum.Totals.Requests != 3 || sum.Totals.Errors != 1 {
		t.Errorf("totals requests/errors = %d/%d, want 3/1", sum.Totals.Requests, sum.Totals.Errors)
	}
	if sum.Totals.In != 110 || sum.Totals.Out != 55 {
		t.Errorf("totals in/out = %d/%d, want 110/55", sum.Totals.In, sum.Totals.Out)
	}
	if sum.Totals.Cached != 20 || sum.Totals.Reasoning != 5 {
		t.Errorf("totals cached/reasoning = %d/%d, want 20/5", sum.Totals.Cached, sum.Totals.Reasoning)
	}
	if sum.Totals.SavedBytes != 7 {
		t.Errorf("saved bytes = %d, want 7", sum.Totals.SavedBytes)
	}
	if sum.Totals.AvgMs != 200 {
		t.Errorf("avg ms = %d, want 200", sum.Totals.AvgMs)
	}
	if len(sum.ByDay) != 7 {
		t.Fatalf("by_day length = %d, want 7", len(sum.ByDay))
	}
	if last := sum.ByDay[len(sum.ByDay)-1]; last.Requests != 3 || last.In != 110 {
		t.Errorf("today bucket = %+v, want 3 requests / 110 in", last)
	}
	if len(sum.ByKey) != 2 {
		t.Fatalf("by_key length = %d, want 2", len(sum.ByKey))
	}
	if sum.ByKey[0].KeyID != "k1" || sum.ByKey[0].Requests != 2 || sum.ByKey[0].Errors != 1 {
		t.Errorf("top key = %+v, want k1 with 2 requests / 1 error", sum.ByKey[0])
	}
	if sum.ByKey[0].LastTS == "" {
		t.Error("expected last_ts on the key row")
	}
	if len(sum.ByModel) != 2 || sum.ByModel[0].Model != "big-pickle" {
		t.Errorf("by_model = %+v, want big-pickle first", sum.ByModel)
	}
	if len(sum.Recent) != 3 {
		t.Fatalf("recent length = %d, want 3", len(sum.Recent))
	}
	if sum.Recent[0].KeyID != "k2" {
		t.Errorf("recent[0] = %q, want the newest event (k2)", sum.Recent[0].KeyID)
	}
}

func TestSummarySpansDaysAndIgnoresOldFiles(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defer svc.Close()

	now := time.Now().UTC()
	svc.Record(Event{TS: now.AddDate(0, 0, -1), KeyID: "k1", Provider: "opencode", Model: "m", Status: 200, In: 1})
	svc.Record(Event{TS: now.AddDate(0, 0, -40), KeyID: "k1", Provider: "opencode", Model: "m", Status: 200, In: 1000})
	svc.Record(Event{TS: now, KeyID: "k1", Provider: "opencode", Model: "m", Status: 200, In: 2})

	files, _ := filepath.Glob(filepath.Join(dir, "usage", "*.jsonl"))
	if len(files) != 3 {
		t.Fatalf("expected 3 day files, got %d", len(files))
	}

	week, err := svc.Summary(7, Filter{})
	if err != nil {
		t.Fatalf("Summary(7): %v", err)
	}
	if week.Totals.Requests != 2 || week.Totals.In != 3 {
		t.Errorf("7-day window = %d requests / %d in, want 2/3 (the 40-day-old file must be ignored)", week.Totals.Requests, week.Totals.In)
	}
	if len(week.ByDay) != 7 {
		t.Errorf("by_day length = %d, want 7", len(week.ByDay))
	}

	recent, err := svc.Summary(90, Filter{})
	if err != nil {
		t.Fatalf("Summary(90): %v", err)
	}
	if recent.Totals.Requests != 3 || recent.Totals.In != 1003 {
		t.Errorf("90-day window = %d requests / %d in, want 3/1003", recent.Totals.Requests, recent.Totals.In)
	}
}

func TestRecordSurvivesBadDirectory(t *testing.T) {
	svc := &Service{dir: filepath.Join(t.TempDir(), "missing", "usage")}
	// A failing write must not panic or block the request path.
	svc.Record(Event{KeyID: "k1", Provider: "opencode", Model: "m", Status: 200})
	if _, err := os.Stat(svc.dir); !os.IsNotExist(err) {
		t.Fatalf("expected no directory creation on the record path, got %v", err)
	}
}

func TestEmptySummaryReturnsArrays(t *testing.T) {
	svc, err := NewService(t.TempDir())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defer svc.Close()

	sum, err := svc.Summary(7, Filter{})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if sum.ByKey == nil || sum.ByModel == nil || sum.Recent == nil || sum.ByDay == nil {
		t.Fatalf("expected empty slices, got by_key=%v by_model=%v recent=%v by_day=%v",
			sum.ByKey, sum.ByModel, sum.Recent, sum.ByDay)
	}

	// The dashboard reads these before the first request exists, so the wire form
	// must be [] and never null.
	b, err := json.Marshal(sum)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, field := range []string{"by_key", "by_model", "recent", "by_day"} {
		if string(raw[field]) == "null" {
			t.Errorf("%s marshalled to null, want []", field)
		}
	}
}

func TestSummaryFilterAndHourlyBuckets(t *testing.T) {
	svc, err := NewService(t.TempDir())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defer svc.Close()

	now := time.Now().UTC()
	svc.Record(Event{TS: now, KeyID: "k1", KeyName: "laptop", Provider: "opencode", Model: "big-pickle", Status: 200, In: 10, Out: 1})
	svc.Record(Event{TS: now, KeyID: "k1", KeyName: "laptop", Provider: "antigravity", Model: "gemini-3.8-flash", Status: 200, In: 20, Out: 2})
	svc.Record(Event{TS: now, KeyID: "k2", KeyName: "ci", Provider: "antigravity", Model: "gemini-3.8-flash", Status: 502, In: 30, Out: 0})

	all, err := svc.Summary(7, Filter{})
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if len(all.ByProvider) != 2 || all.ByProvider[0].Provider != "antigravity" || all.ByProvider[0].Requests != 2 {
		t.Errorf("by_provider = %+v, want antigravity first with 2 requests", all.ByProvider)
	}
	if all.ByHour != nil {
		t.Errorf("expected no hourly buckets for a 7-day window, got %d", len(all.ByHour))
	}

	byProv, err := svc.Summary(7, Filter{Provider: "antigravity"})
	if err != nil {
		t.Fatalf("Summary(prov): %v", err)
	}
	if byProv.Totals.Requests != 2 || byProv.Totals.In != 50 || byProv.Filters.Provider != "antigravity" {
		t.Errorf("provider filter = %+v, want 2 requests / 50 in", byProv.Totals)
	}
	if len(byProv.ByKey) != 2 {
		t.Errorf("by_key under provider filter = %d, want 2", len(byProv.ByKey))
	}

	byKey, err := svc.Summary(7, Filter{Provider: "antigravity", KeyID: "k1"})
	if err != nil {
		t.Fatalf("Summary(key): %v", err)
	}
	if byKey.Totals.Requests != 1 || byKey.Totals.In != 20 {
		t.Errorf("key filter = %+v, want 1 request / 20 in", byKey.Totals)
	}

	byModel, err := svc.Summary(7, Filter{Model: "big-pickle"})
	if err != nil {
		t.Fatalf("Summary(model): %v", err)
	}
	if byModel.Totals.Requests != 1 || len(byModel.ByProvider) != 1 || byModel.ByProvider[0].Provider != "opencode" {
		t.Errorf("model filter = %+v / %+v, want only the opencode request", byModel.Totals, byModel.ByProvider)
	}

	day, err := svc.Summary(1, Filter{})
	if err != nil {
		t.Fatalf("Summary(1): %v", err)
	}
	if len(day.ByHour) != 24 {
		t.Fatalf("by_hour length = %d, want 24", len(day.ByHour))
	}
	hour := day.ByHour[now.Hour()]
	if hour.Requests != 3 || hour.In != 60 {
		t.Errorf("hour bucket %d = %+v, want 3 requests / 60 in", now.Hour(), hour)
	}
}
