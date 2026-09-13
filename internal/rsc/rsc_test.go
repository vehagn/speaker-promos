package rsc

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func loadFlight(t *testing.T) string {
	t.Helper()
	html, err := os.ReadFile("testdata/mini.html")
	if err != nil {
		t.Fatal(err)
	}
	flight, err := Chunks(string(html))
	if err != nil {
		t.Fatal(err)
	}
	return flight
}

func TestChunksRejectsNonRSCPage(t *testing.T) {
	if _, err := Chunks("<html><body>nothing here</body></html>"); err == nil {
		t.Fatal("want error for a page with no flight payload, got nil")
	}
}

func TestChunksReassemblesSplitPayload(t *testing.T) {
	flight := loadFlight(t)
	// The fixture splits the stream mid-row across five push() calls; if they
	// were not concatenated in order, these rows would not be intact.
	for _, want := range []string{"0:{\"a\":1}", "Pods on Mars", "Cloud Native Days Norway 2026"} {
		if !strings.Contains(flight, want) {
			t.Errorf("reassembled flight missing %q", want)
		}
	}
}

// The text row's declared length is in bytes but its content is Norwegian, so a
// rune-based slice would cut it short and desynchronise every later row.
func TestRowsHonoursByteLengthOnMultibyteText(t *testing.T) {
	rows := Rows(loadFlight(t))

	got := rows["37"]
	want := "åpen kildekode og norsk suverenitet.\nAndre avsnitt med æ, ø, å."
	if got != want {
		t.Errorf("row 37:\n got %q\nwant %q", got, want)
	}
	if !strings.Contains(got, "\n") {
		t.Error("row 37 should have kept its embedded newline")
	}
	// Rows after the multi-byte text row must still be found.
	if _, ok := rows["50"]; !ok {
		t.Error("row 50 missing: byte/rune desync swallowed later rows")
	}
	if _, ok := rows["60"]; !ok {
		t.Error("row 60 missing: byte/rune desync swallowed later rows")
	}
}

func TestRowsDoesNotSwallowTypeTags(t *testing.T) {
	// An `I[…]` module row must keep its leading "I" — only `T` is a length
	// prefix and only `T` may be consumed.
	rows := Rows("1:I[\"mod\"]\n2:{\"k\":1}\n")
	if got := rows["1"]; got != `I["mod"]` {
		t.Errorf("row 1 = %q, want %q", got, `I["mod"]`)
	}
	if got := rows["2"]; got != `{"k":1}` {
		t.Errorf("row 2 = %q", got)
	}
}

func TestFindObjectDisambiguatesByRequiredKeys(t *testing.T) {
	flight := loadFlight(t)

	// "schedules" sits in the innermost object, so no requirements are needed.
	raw, err := FindObject(flight, `"schedules":`)
	if err != nil {
		t.Fatal(err)
	}
	var props struct {
		Schedules []struct {
			Date   string
			Tracks []struct {
				TrackTitle string
				Talks      []json.RawMessage
			}
		}
	}
	if err := json.Unmarshal(raw, &props); err != nil {
		t.Fatal(err)
	}
	if len(props.Schedules) != 1 || props.Schedules[0].Date != "2026-10-26" {
		t.Fatalf("schedules = %+v", props.Schedules)
	}

	// The conference object is several levels above the date string used to find
	// it; without `require` the innermost enclosing object would be returned.
	raw, err = FindObject(flight, `"startDate":"2026-10-26"`, "title", "startDate", "city")
	if err != nil {
		t.Fatal(err)
	}
	var conf struct{ Title, StartDate, City string }
	if err := json.Unmarshal(raw, &conf); err != nil {
		t.Fatal(err)
	}
	if conf.Title != "Cloud Native Days Norway 2026" || conf.City != "Bergen" {
		t.Errorf("conference = %+v", conf)
	}
}

func TestFindObjectMissingMarker(t *testing.T) {
	if _, err := FindObject(`{"a":1}`, `"nope":`); err == nil {
		t.Fatal("want error for absent marker")
	}
}

func TestResolveSubstitutesOnlyKnownRows(t *testing.T) {
	rows := map[string]string{"37": "the real text"}
	var v any
	input := `{"description":"$37","mod":"$L54","missing":"$undefined","marker":["$","div"],"n":1,"plain":"$37x"}`
	if err := json.Unmarshal([]byte(input), &v); err != nil {
		t.Fatal(err)
	}
	out := Resolve(v, rows).(map[string]any)

	if out["description"] != "the real text" {
		t.Errorf("description = %v, want resolved", out["description"])
	}
	// Non-row references must survive verbatim — blanking them would destroy
	// element markers and turn absent fields into empty strings.
	for key, want := range map[string]string{
		"mod":     "$L54",
		"missing": "$undefined",
		"plain":   "$37x",
	} {
		if out[key] != want {
			t.Errorf("%s = %v, want %q left verbatim", key, out[key], want)
		}
	}
	if m := out["marker"].([]any); m[0] != "$" || m[1] != "div" {
		t.Errorf("marker = %v", m)
	}
	if out["n"] != float64(1) {
		t.Errorf("n = %v", out["n"])
	}
}

func TestResolveNestedStructures(t *testing.T) {
	rows := map[string]string{"9": "deep"}
	var v any
	if err := json.Unmarshal([]byte(`{"a":[{"b":["$9"]}]}`), &v); err != nil {
		t.Fatal(err)
	}
	got := Resolve(v, rows).(map[string]any)["a"].([]any)[0].(map[string]any)["b"].([]any)[0]
	if got != "deep" {
		t.Errorf("nested resolve = %v, want deep", got)
	}
}
