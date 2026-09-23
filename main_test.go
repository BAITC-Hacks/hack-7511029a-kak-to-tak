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

func TestWishRanksAndOffersAlternatives(t *testing.T) {
	q := Query{City: "Алматы", Category: "Ведущий", Format: "корпоратив", Date: "2026-10-15", Budget: 500000, Wish: "спокойный интеллигентный ведущий"}
	base := Contractor{ID: "plain", Name: "Обычный", City: q.City, Categories: []string{q.Category}, Formats: []string{q.Format}, Price: 300000, Description: "Проводит мероприятия."}
	meaningful := base
	meaningful.ID, meaningful.Name, meaningful.Description = "meaningful", "Спокойный", "Интеллигентный ведущий создаёт комфортную атмосферу для гостей."
	busy := meaningful
	busy.ID, busy.Name, busy.BusyDates = "busy", "Занят", []string{q.Date}
	over := meaningful
	over.ID, over.Name, over.Price = "over", "Дороже", 600000
	r, err := recommend([]Contractor{base, meaningful, busy, over}, q)
	if err != nil {
		t.Fatal(err)
	}
	if r.Cards[0].Contractor.ID != "meaningful" || len(r.Cards[0].Evidence) == 0 {
		t.Fatalf("wish was not ranked: %+v", r.Cards)
	}
	if len(r.Alternatives) != 2 {
		t.Fatalf("alternatives: %+v", r.Alternatives)
	}
}

func TestBundleMatchingIsDeterministicAndWithinBudget(t *testing.T) {
	q := BundleQuery{City: "Алматы", Date: "2026-10-15", Format: "свадьба", Categories: []string{"Ведущий", "Фотограф", "Танцевальный коллектив"}, Budget: 1000000, Durations: map[string]int{"Ведущий": 8, "Фотограф": 5, "Танцевальный коллектив": 2}}
	makeC := func(id, category string, price int) Contractor {
		return Contractor{ID: id, Name: id, City: q.City, Categories: []string{category}, Formats: []string{q.Format}, Price: price}
	}
	catalog := []Contractor{makeC("host", "Ведущий", 300000), makeC("photo", "Фотограф", 300000), makeC("dance", "Танцевальный коллектив", 300000)}
	one, err := recommendBundle(catalog, q)
	if err != nil || one.Status != "complete" || len(one.Items) != 3 || one.Total != 900000 {
		t.Fatalf("unexpected bundle: %+v, %v", one, err)
	}
	two, _ := recommendBundle(catalog, q)
	if one.Items[0].Contractor.ID != two.Items[0].Contractor.ID || one.Total != two.Total {
		t.Fatal("bundle ordering is not deterministic")
	}
}

func TestBundleUsesDurationPerCategoryAndAllowsUnlimited(t *testing.T) {
	q := BundleQuery{City: "Алматы", Date: "2026-10-15", Format: "свадьба", Categories: []string{"Отель", "Ведущий"}, Budget: 1000000, Durations: map[string]int{"Отель": 12, "Ведущий": 8}}
	hotel := Contractor{ID: "hotel", Name: "Hotel", City: q.City, Categories: []string{"Отель"}, Formats: []string{q.Format}, Price: 400000}
	hotelHours := 10
	hotel.MaxHours = &hotelHours
	host := Contractor{ID: "host", Name: "Host", City: q.City, Categories: []string{"Ведущий"}, Formats: []string{q.Format}, Price: 300000}
	short := 6
	host.MaxHours = &short
	unlimited := Contractor{ID: "unlimited", Name: "Unlimited", City: q.City, Categories: []string{"Отель"}, Formats: []string{q.Format}, Price: 450000}
	r, err := recommendBundle([]Contractor{hotel, host, unlimited}, q)
	if err != nil || len(r.Items) != 1 || r.Items[0].Contractor.ID != "unlimited" {
		t.Fatalf("duration filtering failed: %+v, %v", r, err)
	}
}

func TestBundleBudgetAlternativeLimit(t *testing.T) {
	q := BundleQuery{City: "Алматы", Date: "2026-10-15", Format: "свадьба", Categories: []string{"Ведущий", "Фотограф"}, Budget: 500000, Durations: map[string]int{"Ведущий": 8, "Фотограф": 5}}
	base := Contractor{City: q.City, Formats: []string{q.Format}}
	host := base
	host.ID, host.Name, host.Categories, host.Price = "host", "Host", []string{"Ведущий"}, 250000
	photo := base
	photo.ID, photo.Name, photo.Categories, photo.Price = "photo", "Photo", []string{"Фотограф"}, 320000
	r, err := recommendBundle([]Contractor{host, photo}, q)
	if err != nil || len(r.Alternatives) != 1 || r.Alternatives[0].Difference != 70000 {
		t.Fatalf("expected +70k alternative: %+v, %v", r, err)
	}
	photo.Price = 650000
	r, _ = recommendBundle([]Contractor{host, photo}, q)
	if len(r.Alternatives) != 0 {
		t.Fatalf("+150k must not be offered: %+v", r.Alternatives)
	}
}
