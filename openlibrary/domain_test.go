package openlibrary

import (
	"testing"
)

// These tests are offline: they exercise the URI driver's pure string functions.
// The client's HTTP behaviour is covered in openlibrary_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "openlibrary" {
		t.Errorf("Scheme = %q, want openlibrary", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "openlibrary" {
		t.Errorf("Identity.Binary = %q, want openlibrary", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		in  string
		typ string
		id  string
	}{
		{"OL34184A", "author", "OL34184A"},
		{"OL45804W", "work", "OL45804W"},
		{"0140328726", "isbn", "0140328726"},
		{"9780140328726", "isbn", "9780140328726"},
		{"dune", "query", "dune"},
		{"frank herbert", "query", "frank herbert"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestClassifyEmpty(t *testing.T) {
	_, _, err := Domain{}.Classify("")
	if err == nil {
		t.Error("Classify(\"\") should return error")
	}
}

func TestLocate(t *testing.T) {
	cases := []struct {
		uriType string
		id      string
		want    string
	}{
		{"author", "OL34184A", "https://openlibrary.org/authors/OL34184A"},
		{"work", "OL45804W", "https://openlibrary.org/works/OL45804W"},
		{"isbn", "0140328726", "https://openlibrary.org/isbn/0140328726"},
		{"query", "dune", "https://openlibrary.org/search?q=dune"},
	}
	for _, tc := range cases {
		got, err := Domain{}.Locate(tc.uriType, tc.id)
		if err != nil || got != tc.want {
			t.Errorf("Locate(%q, %q) = (%q, %v), want (%q, nil)",
				tc.uriType, tc.id, got, err, tc.want)
		}
	}
}

func TestLocateUnknown(t *testing.T) {
	_, err := Domain{}.Locate("unknown", "foo")
	if err == nil {
		t.Error("Locate with unknown type should return error")
	}
}

func TestFlattenText(t *testing.T) {
	raw := func(s string) []byte { return []byte(s) }

	cases := []struct {
		input string
		want  string
	}{
		{`"plain string"`, "plain string"},
		{`{"type":"/type/text","value":"nested value"}`, "nested value"},
		{`""`, ""},
		{`{}`, ""},
	}
	for _, tc := range cases {
		got := flattenText(raw(tc.input))
		if got != tc.want {
			t.Errorf("flattenText(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestStripPrefixes(t *testing.T) {
	if got := stripWorksPrefix("/works/OL45804W"); got != "OL45804W" {
		t.Errorf("stripWorksPrefix = %q", got)
	}
	if got := stripWorksPrefix("OL45804W"); got != "OL45804W" {
		t.Errorf("stripWorksPrefix no-op = %q", got)
	}
	if got := stripAuthorsPrefix("/authors/OL34184A"); got != "OL34184A" {
		t.Errorf("stripAuthorsPrefix = %q", got)
	}
	if got := stripAuthorsPrefix("OL34184A"); got != "OL34184A" {
		t.Errorf("stripAuthorsPrefix no-op = %q", got)
	}
}
