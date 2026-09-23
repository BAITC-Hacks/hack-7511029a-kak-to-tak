package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed web/*
var webFiles embed.FS

func webHandler(catalog []Contractor) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/options", func(w http.ResponseWriter, r *http.Request) {
		options := map[string][]string{}
		sets := map[string]map[string]bool{}
		for _, c := range catalog {
			for key, values := range map[string][]string{"cities": {c.City}, "categories": c.Categories, "formats": c.Formats, "languages": c.Languages} {
				if sets[key] == nil {
					sets[key] = map[string]bool{}
				}
				for _, v := range values {
					sets[key][v] = true
				}
			}
		}
		for key, values := range sets {
			for value := range values {
				options[key] = append(options[key], value)
			}
			sort.Strings(options[key])
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(options)
	})
	mux.HandleFunc("/api/recommend", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Используйте GET"})
			return
		}
		v := r.URL.Query()
		budget, err := strconv.Atoi(v.Get("budget"))
		if err != nil {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": "Введите бюджет целым числом."})
			return
		}
		hours := 0
		if v.Get("hours") != "" {
			hours, err = strconv.Atoi(v.Get("hours"))
			if err != nil {
				w.WriteHeader(400)
				json.NewEncoder(w).Encode(map[string]string{"error": "Введите длительность целым числом часов."})
				return
			}
		}
		result, err := recommend(catalog, Query{City: v.Get("city"), Category: v.Get("category"), Date: v.Get("date"), Format: v.Get("format"), Language: v.Get("language"), Wish: v.Get("wish"), Budget: budget, Hours: hours})
		if err != nil {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(result)
	})
	mux.HandleFunc("/api/bundle", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Используйте GET"})
			return
		}
		v := r.URL.Query()
		budget, err := strconv.Atoi(v.Get("budget"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Введите общий бюджет целым числом."})
			return
		}
		categories := v["category"]
		if len(categories) == 1 {
			categories = strings.Split(categories[0], ",")
		}
		for i := range categories {
			categories[i] = strings.TrimSpace(categories[i])
		}
		durations := map[string]int{}
		for _, raw := range v["duration"] {
			parts := strings.SplitN(raw, "|", 2)
			if len(parts) != 2 {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": "Укажите часы для каждой выбранной категории."})
				return
			}
			hours, parseErr := strconv.Atoi(parts[1])
			if parseErr != nil || hours < 0 {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": "Часы для категории должны быть неотрицательным числом."})
				return
			}
			durations[strings.TrimSpace(parts[0])] = hours
		}
		result, err := recommendBundle(catalog, BundleQuery{City: v.Get("city"), Date: v.Get("date"), Format: v.Get("format"), Language: v.Get("language"), Categories: categories, Budget: budget, Durations: durations})
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(result)
	})
	mux.HandleFunc("/api/ai/parse", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Используйте POST"})
			return
		}
		var input AIRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&input); err != nil || strings.TrimSpace(input.Text) == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Введите текстовый запрос."})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()
		parsed, err := callOpenAIParser(ctx, catalog, input.Text, input.Context)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(AIResponse{Parsed: &parsed})
	})
	mux.HandleFunc("/api/ai/voice", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			json.NewEncoder(w).Encode(map[string]string{"error": "Используйте POST"})
			return
		}
		if r.ContentLength > 10<<20 {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			json.NewEncoder(w).Encode(map[string]string{"error": "Аудиофайл слишком большой (максимум 10 МБ)."})
			return
		}
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Не удалось прочитать аудиофайл."})
			return
		}
		file, header, err := r.FormFile("audio")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Аудиофайл не найден."})
			return
		}
		defer file.Close()
		if header.Size == 0 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Аудиофайл пустой."})
			return
		}
		if !strings.HasPrefix(header.Header.Get("Content-Type"), "audio/") {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Поддерживаются только аудиофайлы."})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		transcript, err := transcribeAudio(ctx, file, header)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		parsed, err := callOpenAIParser(ctx, catalog, transcript, nil)
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(AIResponse{Transcript: transcript, Parsed: &parsed})
	})
	assets, _ := fs.Sub(webFiles, "web")
	mux.Handle("/", http.FileServer(http.FS(assets)))
	return mux
}

func serveWeb(path, addr string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	catalog, err := loadCatalog(f)
	if err != nil {
		return err
	}
	fmt.Printf("Откройте в браузере http://%s\nКаталог: %d профилей. Для остановки нажмите Ctrl+C.\n", addr, len(catalog))
	s := &http.Server{Addr: addr, Handler: webHandler(catalog), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	return s.ListenAndServe()
}
