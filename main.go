package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Contractor struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	City         string   `json:"city"`
	Categories   []string `json:"categories"`
	Price        int      `json:"price_from_kzt"`
	Formats      []string `json:"event_formats"`
	Languages    []string `json:"languages"`
	MaxHours     *int     `json:"max_hours"`
	BusyDates    []string `json:"busy_dates"`
	Description  string   `json:"description"`
	Synthetic    bool     `json:"synthetic"`
	CityImputed  bool     `json:"city_imputed"`
	PriceImputed bool     `json:"price_imputed"`
}

type Query struct {
	City, Category, Date, Format, Language string
	Budget, Hours                          int
}
type Card struct {
	Contractor  Contractor `json:"contractor"`
	Explanation string     `json:"explanation"`
}
type Result struct {
	Status   string         `json:"status"`
	Message  string         `json:"message"`
	Eligible int            `json:"eligible_count"`
	Rejected map[string]int `json:"rejection_counts"`
	Cards    []Card         `json:"cards"`
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func loadCatalog(reader io.Reader) ([]Contractor, error) {
	r := csv.NewReader(reader)
	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	cols := map[string]int{}
	for i, name := range header {
		cols[strings.TrimPrefix(name, "\ufeff")] = i
	}
	for _, name := range []string{"id", "anon_name", "city", "categories", "price_from_kzt", "event_formats", "languages", "max_hours", "busy_dates", "description", "synthetic", "city_imputed", "price_imputed"} {
		if _, ok := cols[name]; !ok {
			return nil, fmt.Errorf("missing column: %s", name)
		}
	}
	var catalog []Contractor
	seen := map[string]bool{}
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		get := func(key string) string { return strings.TrimSpace(row[cols[key]]) }
		c := Contractor{ID: get("id"), Name: get("anon_name"), City: get("city"), Categories: strings.Split(get("categories"), "|"), Formats: strings.Split(get("event_formats"), "|"), Languages: strings.Split(get("languages"), "|"), Description: get("description")}
		if c.ID == "" || seen[c.ID] {
			return nil, fmt.Errorf("empty or duplicate id: %s", c.ID)
		}
		seen[c.ID] = true
		c.Price, err = strconv.Atoi(get("price_from_kzt"))
		if err != nil || c.Price < 0 {
			return nil, fmt.Errorf("invalid price: %s", c.ID)
		}
		if get("max_hours") != "" {
			n, err := strconv.Atoi(get("max_hours"))
			if err != nil || n <= 0 {
				return nil, fmt.Errorf("invalid hours: %s", c.ID)
			}
			c.MaxHours = &n
		}
		for key, target := range map[string]*bool{"synthetic": &c.Synthetic, "city_imputed": &c.CityImputed, "price_imputed": &c.PriceImputed} {
			v, err := strconv.ParseBool(get(key))
			if err != nil {
				return nil, fmt.Errorf("invalid %s: %s", key, c.ID)
			}
			*target = v
		}
		if get("busy_dates") != "" {
			c.BusyDates = strings.Split(get("busy_dates"), "|")
		}
		for _, date := range c.BusyDates {
			if _, err := time.Parse("2006-01-02", date); err != nil {
				return nil, fmt.Errorf("invalid busy date: %s", c.ID)
			}
		}
		catalog = append(catalog, c)
	}
	return catalog, nil
}

func recommend(catalog []Contractor, q Query) (Result, error) {
	r := Result{Cards: []Card{}, Rejected: map[string]int{}}
	if q.City == "" || q.Category == "" || q.Format == "" || q.Budget <= 0 || q.Hours < 0 {
		return r, fmt.Errorf("город, категория, формат и положительный бюджет обязательны; длительность не может быть отрицательной")
	}
	if _, err := time.Parse("2006-01-02", q.Date); err != nil {
		return r, fmt.Errorf("дата должна быть в формате YYYY-MM-DD")
	}
	if q.Date < "2026-09-23" || q.Date > "2026-12-31" {
		return r, fmt.Errorf("календарь доступен только с 2026-09-23 по 2026-12-31")
	}
	candidates := 0
	var eligible []Contractor
	for _, c := range catalog {
		if c.City != q.City || !contains(c.Categories, q.Category) {
			continue
		}
		candidates++
		// Count every failed condition; counts can overlap.
		failed := false
		reject := func(reason string, condition bool) {
			if condition {
				r.Rejected[reason]++
				failed = true
			}
		}
		reject("busy", contains(c.BusyDates, q.Date))
		reject("budget", c.Price > q.Budget)
		reject("format", !contains(c.Formats, q.Format))
		reject("language", q.Language != "" && !contains(c.Languages, q.Language))
		reject("duration", q.Hours > 0 && c.MaxHours != nil && q.Hours > *c.MaxHours)
		if !failed {
			eligible = append(eligible, c)
		}
	}
	if candidates == 0 {
		r.Status = "category_absent"
		r.Message = "В этом городе нет подрядчиков выбранной категории."
		return r, nil
	}
	if len(eligible) == 0 {
		r.Status = "no_match"
		r.Message = "Кандидаты есть, но никто не проходит все условия. Причины исключения указаны в rejection_counts; один профиль может иметь несколько причин."
		return r, nil
	}
	// Baseline only: deterministic ordering, not semantic ranking.
	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].Price != eligible[j].Price {
			return eligible[i].Price < eligible[j].Price
		}
		return eligible[i].ID < eligible[j].ID
	})
	r.Status = "matched"
	r.Eligible = len(eligible)
	r.Message = fmt.Sprintf("Подходящих профилей: %d. Показано до трёх по начальной цене; итоговую стоимость нужно уточнить.", len(eligible))
	if len(eligible) < 3 {
		r.Message += fmt.Sprintf(" Всего в городе и категории: %d; остальные исключены по условиям (причины могут пересекаться).", candidates)
	}
	for i, c := range eligible {
		if i == 3 {
			break
		}
		text := fmt.Sprintf("На %s занятость не указана; профиль принимает формат «%s», начальная цена %d ₸ при бюджете %d ₸.", q.Date, q.Format, c.Price, q.Budget)
		if q.Language != "" {
			text += " Язык работы: " + q.Language + "."
		}
		r.Cards = append(r.Cards, Card{Contractor: c, Explanation: text})
	}
	return r, nil
}

func main() {
	var q Query
	path := flag.String("data", "data/contractors.csv", "CSV каталога")
	flag.StringVar(&q.City, "city", "", "город")
	flag.StringVar(&q.Category, "category", "", "категория")
	flag.StringVar(&q.Date, "date", "", "YYYY-MM-DD")
	flag.StringVar(&q.Format, "format", "", "формат мероприятия")
	flag.StringVar(&q.Language, "language", "", "язык (необязательно)")
	flag.IntVar(&q.Budget, "budget", 0, "бюджет в тенге")
	flag.IntVar(&q.Hours, "hours", 0, "длительность (необязательно)")
	flag.Parse()
	err := run(*path, q)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(path string, q Query) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	catalog, err := loadCatalog(f)
	if err != nil {
		return err
	}
	result, err := recommend(catalog, q)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}
