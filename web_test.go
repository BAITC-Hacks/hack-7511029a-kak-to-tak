package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestWebFlow(t *testing.T) {
	f, err := os.Open("data/contractors.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	catalog, err := loadCatalog(f)
	if err != nil {
		t.Fatal(err)
	}
	h := webHandler(catalog)
	for _, tt := range []struct {
		url    string
		code   int
		status string
	}{
		{"/", 200, ""},
		{"/api/options", 200, ""},
		{"/api/recommend?city=Алматы&category=Ведущий&date=2026-10-15&format=корпоратив&budget=1000000", 200, "matched"},
		{"/api/recommend?city=Алматы&category=Ведущий&date=2026-10-15&format=корпоратив&budget=1000", 200, "no_match"},
		{"/api/recommend?city=Зарубежье&category=Флорист&date=2026-10-15&format=свадьба&budget=1000000", 200, "category_absent"},
		{"/api/recommend?city=Алматы&category=Ведущий&date=2027-10-15&format=корпоратив&budget=1000000", 400, ""},
		{"/api/recommend?budget=wrong", 400, ""},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", tt.url, nil))
		if w.Code != tt.code {
			t.Fatalf("%s: %d %s", tt.url, w.Code, w.Body.String())
		}
		if tt.url == "/" && !strings.Contains(w.Body.String(), "Ваше событие") {
			t.Fatal("page missing")
		}
		if tt.status != "" {
			var result Result
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Status != tt.status {
				t.Fatalf("%s: %s", tt.url, result.Status)
			}
			if tt.status == "matched" && len(result.Cards) != 3 {
				t.Fatal("expected three cards")
			}
		}
	}
}
