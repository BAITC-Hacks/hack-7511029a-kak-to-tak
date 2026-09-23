package main

import (
	"os"
	"testing"
)

func TestSourceCatalog(t *testing.T) {
	f, err := os.Open("data/contractors.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	c, err := loadCatalog(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(c) != 66 {
		t.Fatalf("profiles: %d", len(c))
	}
}

func TestRecommendationConstraints(t *testing.T) {
	q := Query{City: "Алматы", Category: "Флорист", Format: "свадьба", Date: "2026-10-01", Budget: 200000, Hours: 8, Language: "русский"}
	base := Contractor{ID: "B", City: q.City, Categories: []string{q.Category}, Formats: []string{q.Format}, Languages: []string{q.Language}, Price: q.Budget}
	busy := base
	busy.ID = "busy"
	busy.BusyDates = []string{q.Date}
	expensive := base
	expensive.ID = "expensive"
	expensive.Price++
	short := base
	short.ID = "short"
	hours := 4
	short.MaxHours = &hours
	other := base
	other.ID = "A"
	r, err := recommend([]Contractor{base, busy, expensive, short, other}, q)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "matched" || len(r.Cards) != 2 || r.Cards[0].Contractor.ID != "A" {
		t.Fatalf("unexpected result: %+v", r)
	}
	for _, key := range []string{"busy", "budget", "duration"} {
		if r.Rejected[key] != 1 {
			t.Fatalf("missing rejection: %s", key)
		}
	}
	r, _ = recommend([]Contractor{busy}, q)
	if r.Status != "no_match" {
		t.Fatal(r.Status)
	}
	r, _ = recommend(nil, q)
	if r.Status != "category_absent" {
		t.Fatal(r.Status)
	}
	q.Date = "2027-01-01"
	if _, err = recommend([]Contractor{base}, q); err == nil {
		t.Fatal("unknown calendar accepted")
	}
}
