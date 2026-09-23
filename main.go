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
	City, Category, Date, Format, Language, Wish string
	Budget, Hours                                int
}
type Card struct {
	Contractor  Contractor `json:"contractor"`
	Explanation string     `json:"explanation"`
	Relevance   int        `json:"relevance"`
	Evidence    []string   `json:"evidence"`
}
type Alternative struct {
	Kind       string     `json:"kind"`
	Contractor Contractor `json:"contractor"`
	Message    string     `json:"message"`
	Date       string     `json:"date,omitempty"`
	Relevance  int        `json:"relevance"`
	Evidence   []string   `json:"evidence"`
}
type Result struct {
	Status       string         `json:"status"`
	Message      string         `json:"message"`
	Eligible     int            `json:"eligible_count"`
	Rejected     map[string]int `json:"rejection_counts"`
	Cards        []Card         `json:"cards"`
	Alternatives []Alternative  `json:"alternatives"`
}

type rankedContractor struct {
	Contractor
	relevance int
	evidence  []string
}

func normalizedTokens(text string) []string {
	words := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !((r >= 'а' && r <= 'я') || (r >= 'А' && r <= 'Я') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'))
	})
	stop := map[string]bool{"и": true, "в": true, "на": true, "с": true, "для": true, "по": true, "что": true, "это": true, "без": true, "или": true, "а": true, "но": true, "мы": true, "мне": true, "нам": true, "нужен": true, "нужна": true, "ищем": true, "хочу": true, "хотим": true, "будет": true, "чтобы": true, "как": true, "от": true}
	seen := map[string]bool{}
	var result []string
	for _, word := range words {
		if len([]rune(word)) < 3 || stop[word] || seen[word] {
			continue
		}
		seen[word] = true
		result = append(result, word)
	}
	return result
}

func matchedEvidence(description, wish string) ([]string, int) {
	terms := normalizedTokens(wish)
	if len(terms) == 0 {
		return nil, 0
	}
	text := strings.ToLower(description)
	// Small transparent synonym map keeps matching meaningful phrases such as "спокойный" and "интеллигентный".
	synonyms := map[string][]string{"спокой": {"интеллигент", "комфорт", "атмосфер", "лампов"}, "энерг": {"динамич", "шоу", "импровиз", "харизм"}, "соврем": {"современн", "креатив", "интерактив"}, "свадеб": {"свадеб", "молодож", "пара"}, "дет": {"детск", "ребен", "семейн"}, "делов": {"бизнес", "конференц", "форум", "корпоратив"}}
	matched := map[string]bool{}
	points := 0
	for _, term := range terms {
		prefix := []rune(term)
		if len(prefix) > 5 {
			prefix = prefix[:5]
		}
		key := string(prefix)
		if strings.Contains(text, key) {
			matched[term] = true
			points += 2
			continue
		}
		for root, words := range synonyms {
			if strings.HasPrefix(term, root) {
				for _, word := range words {
					if strings.Contains(text, word) {
						matched[term] = true
						points++
						break
					}
				}
			}
		}
	}
	if len(matched) == 0 {
		return nil, 0
	}
	for _, sentence := range strings.FieldsFunc(description, func(r rune) bool { return r == '.' || r == '!' || r == '?' || r == '\n' }) {
		lower := strings.ToLower(sentence)
		for term := range matched {
			prefix := []rune(term)
			if len(prefix) > 5 {
				prefix = prefix[:5]
			}
			if strings.Contains(lower, string(prefix)) {
				trimmed := strings.TrimSpace(sentence)
				if len([]rune(trimmed)) > 22 {
					return []string{trimmed}, points
				}
			}
		}
	}
	return []string{"В описании профиля есть совпадение с вашим пожеланием."}, points
}

func nearestAvailableDate(c Contractor, selected string) string {
	start, err := time.Parse("2006-01-02", selected)
	if err != nil {
		return ""
	}
	for offset := 1; offset <= 45; offset++ {
		for _, direction := range []int{1, -1} {
			candidate := start.AddDate(0, 0, offset*direction)
			formatted := candidate.Format("2006-01-02")
			if formatted >= "2026-09-23" && formatted <= "2026-12-31" && !contains(c.BusyDates, formatted) {
				return formatted
			}
		}
	}
	return ""
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
	r := Result{Cards: []Card{}, Alternatives: []Alternative{}, Rejected: map[string]int{}}
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
	var eligible []rankedContractor
	var overBudget, unavailable []rankedContractor
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
		evidence, relevance := matchedEvidence(c.Description, q.Wish)
		if !failed {
			eligible = append(eligible, rankedContractor{c, relevance, evidence})
		}
		// Alternatives are intentionally separate and relax exactly one business constraint.
		baseFit := !(!contains(c.Formats, q.Format) || (q.Language != "" && !contains(c.Languages, q.Language)) || (q.Hours > 0 && c.MaxHours != nil && q.Hours > *c.MaxHours))
		if baseFit && !contains(c.BusyDates, q.Date) && c.Price > q.Budget && c.Price <= q.Budget+max(200000, q.Budget/4) {
			overBudget = append(overBudget, rankedContractor{c, relevance, evidence})
		}
		if baseFit && c.Price <= q.Budget && contains(c.BusyDates, q.Date) {
			unavailable = append(unavailable, rankedContractor{c, relevance, evidence})
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
	} else {
		// The user's wording has priority; price and ID make equal scores deterministic.
		sort.Slice(eligible, func(i, j int) bool {
			if eligible[i].relevance != eligible[j].relevance {
				return eligible[i].relevance > eligible[j].relevance
			}
			if eligible[i].Price != eligible[j].Price {
				return eligible[i].Price < eligible[j].Price
			}
			return eligible[i].ID < eligible[j].ID
		})
		r.Status = "matched"
		r.Eligible = len(eligible)
		r.Message = fmt.Sprintf("Подходящих профилей: %d. Показано до трёх по смысловому соответствию и начальной цене; итоговую стоимость нужно уточнить.", len(eligible))
		if len(eligible) < 3 {
			r.Message += fmt.Sprintf(" Всего в городе и категории: %d; остальные исключены по условиям (причины могут пересекаться).", candidates)
		}
		for i, ranked := range eligible {
			if i == 3 {
				break
			}
			c := ranked.Contractor
			text := fmt.Sprintf("На %s занятость не указана; профиль принимает формат «%s», начальная цена %d ₸ при бюджете %d ₸.", q.Date, q.Format, c.Price, q.Budget)
			if q.Language != "" {
				text += " Язык работы: " + q.Language + "."
			}
			if len(ranked.evidence) > 0 {
				text = "В описании есть совпадение с вашим пожеланием: «" + ranked.evidence[0] + "». " + text
			}
			r.Cards = append(r.Cards, Card{Contractor: c, Explanation: text, Relevance: ranked.relevance, Evidence: ranked.evidence})
		}
	}
	sort.Slice(overBudget, func(i, j int) bool {
		if overBudget[i].relevance != overBudget[j].relevance {
			return overBudget[i].relevance > overBudget[j].relevance
		}
		return overBudget[i].Price < overBudget[j].Price
	})
	for i, candidate := range overBudget {
		if i == 2 {
			break
		}
		extra := candidate.Price - q.Budget
		r.Alternatives = append(r.Alternatives, Alternative{Kind: "over_budget", Contractor: candidate.Contractor, Relevance: candidate.relevance, Evidence: candidate.evidence, Message: fmt.Sprintf("Свободен на %s и подходит по остальным условиям, но начальная цена выше бюджета на %d ₸.", q.Date, extra)})
	}
	sort.Slice(unavailable, func(i, j int) bool {
		if unavailable[i].relevance != unavailable[j].relevance {
			return unavailable[i].relevance > unavailable[j].relevance
		}
		return unavailable[i].Price < unavailable[j].Price
	})
	for _, candidate := range unavailable {
		if len(r.Alternatives) >= 4 {
			break
		}
		nearest := nearestAvailableDate(candidate.Contractor, q.Date)
		if nearest != "" {
			r.Alternatives = append(r.Alternatives, Alternative{Kind: "other_date", Contractor: candidate.Contractor, Date: nearest, Relevance: candidate.relevance, Evidence: candidate.evidence, Message: fmt.Sprintf("На выбранную дату занят. Ближайшая свободная дата по календарю: %s.", nearest)})
		}
	}
	return r, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func main() {
	var q Query
	path := flag.String("data", "data/contractors.csv", "CSV каталога")
	serve := flag.Bool("serve", false, "открыть веб-сервер")
	addr := flag.String("addr", "127.0.0.1:8080", "адрес веб-сервера")
	flag.StringVar(&q.City, "city", "", "город")
	flag.StringVar(&q.Category, "category", "", "категория")
	flag.StringVar(&q.Date, "date", "", "YYYY-MM-DD")
	flag.StringVar(&q.Format, "format", "", "формат мероприятия")
	flag.StringVar(&q.Language, "language", "", "язык (необязательно)")
	flag.StringVar(&q.Wish, "wish", "", "пожелание своими словами (необязательно)")
	flag.IntVar(&q.Budget, "budget", 0, "бюджет в тенге")
	flag.IntVar(&q.Hours, "hours", 0, "длительность (необязательно)")
	flag.Parse()
	var err error
	if *serve || flag.NFlag() == 0 {
		err = serveWeb(*path, *addr)
	} else {
		err = run(*path, q)
	}
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
