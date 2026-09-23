package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"
)

type AIRequest struct {
	Text    string          `json:"text"`
	Context json.RawMessage `json:"context,omitempty"`
}

type ParsedRequest struct {
	Mode              string         `json:"mode"`
	City              string         `json:"city,omitempty"`
	Date              string         `json:"date,omitempty"`
	EventFormat       string         `json:"eventFormat,omitempty"`
	Category          string         `json:"category,omitempty"`
	Categories        []string       `json:"categories,omitempty"`
	BudgetKzt         int            `json:"budgetKzt,omitempty"`
	TotalBudgetKzt    int            `json:"totalBudgetKzt,omitempty"`
	Language          string         `json:"language,omitempty"`
	DurationHours     int            `json:"durationHours,omitempty"`
	CategoryDurations map[string]int `json:"categoryDurations,omitempty"`
	Preferences       string         `json:"preferences,omitempty"`
	Missing           []string       `json:"missing,omitempty"`
}

type AIResponse struct {
	Transcript string         `json:"transcript,omitempty"`
	Parsed     *ParsedRequest `json:"parsed,omitempty"`
	Error      string         `json:"error,omitempty"`
}

func aiModel() string {
	if model := os.Getenv("OPENAI_MODEL"); model != "" {
		return model
	}
	return "gpt-4o-mini"
}

func transcribeModel() string {
	if model := os.Getenv("OPENAI_TRANSCRIBE_MODEL"); model != "" {
		return model
	}
	return "gpt-4o-mini-transcribe"
}

func catalogEnums(catalog []Contractor) (map[string][]string, error) {
	sets := map[string]map[string]bool{"city": {}, "category": {}, "format": {}, "language": {}}
	for _, c := range catalog {
		sets["city"][c.City] = true
		for _, v := range c.Categories {
			sets["category"][v] = true
		}
		for _, v := range c.Formats {
			sets["format"][v] = true
		}
		for _, v := range c.Languages {
			sets["language"][v] = true
		}
	}
	result := map[string][]string{}
	for key, values := range sets {
		for value := range values {
			result[key] = append(result[key], value)
		}
		sortStrings(result[key])
	}
	return result, nil
}

func sortStrings(values []string) {
	for i := range values {
		for j := i + 1; j < len(values); j++ {
			if values[j] < values[i] {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}

func parseAIResponse(body []byte) (ParsedRequest, error) {
	var envelope struct {
		Output []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ParsedRequest{}, err
	}
	for _, item := range envelope.Output {
		for _, content := range item.Content {
			var parsed ParsedRequest
			if json.Unmarshal([]byte(content.Text), &parsed) == nil {
				return parsed, nil
			}
		}
	}
	return ParsedRequest{}, fmt.Errorf("OpenAI вернул ответ без структурированных параметров")
}

func validateParsedRequest(parsed ParsedRequest, catalog []Contractor) error {
	if parsed.Mode != "single" && parsed.Mode != "bundle" {
		return fmt.Errorf("structured response содержит неизвестный режим")
	}
	enums, _ := catalogEnums(catalog)
	valid := func(value string, values []string) bool {
		if value == "" {
			return true
		}
		for _, item := range values {
			if value == item {
				return true
			}
		}
		return false
	}
	if !valid(parsed.City, enums["city"]) || !valid(parsed.EventFormat, enums["format"]) || !valid(parsed.Language, enums["language"]) {
		return fmt.Errorf("structured response содержит значение вне справочника")
	}
	for _, category := range append([]string{parsed.Category}, parsed.Categories...) {
		if !valid(category, enums["category"]) {
			return fmt.Errorf("structured response содержит неизвестную категорию")
		}
	}
	if parsed.BudgetKzt < 0 || parsed.TotalBudgetKzt < 0 || parsed.DurationHours < 0 {
		return fmt.Errorf("structured response содержит отрицательное число")
	}
	for category, hours := range parsed.CategoryDurations {
		if !valid(category, enums["category"]) || hours < 0 {
			return fmt.Errorf("structured response содержит неверную длительность")
		}
	}
	return nil
}

func callOpenAIParser(ctx context.Context, catalog []Contractor, text string, previous json.RawMessage) (ParsedRequest, error) {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		return ParsedRequest{}, fmt.Errorf("OPENAI_API_KEY не настроен; заполните форму вручную")
	}
	enums, _ := catalogEnums(catalog)
	schema := map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{
		"mode": map[string]any{"type": "string", "enum": []string{"single", "bundle"}}, "city": map[string]any{"type": "string"}, "date": map[string]any{"type": "string"}, "eventFormat": map[string]any{"type": "string"}, "category": map[string]any{"type": "string"}, "categories": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "budgetKzt": map[string]any{"type": "integer"}, "totalBudgetKzt": map[string]any{"type": "integer"}, "language": map[string]any{"type": "string"}, "durationHours": map[string]any{"type": "integer"}, "categoryDurations": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "integer"}}, "preferences": map[string]any{"type": "string"}, "missing": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	}, "required": []string{"mode", "city", "date", "eventFormat", "category", "categories", "budgetKzt", "totalBudgetKzt", "language", "durationHours", "categoryDurations", "preferences", "missing"}}
	prompt := fmt.Sprintf("Преобразуй запрос пользователя в JSON. Не придумывай значения вне справочников. Дата YYYY-MM-DD, деньги в KZT. Для неполных запросов заполни missing и сохрани известные поля. Справочники: %s\nКонтекст: %s\nЗапрос: %s", mustJSON(enums), string(previous), text)
	payload := map[string]any{"model": aiModel(), "input": prompt, "temperature": 0, "text": map[string]any{"format": map[string]any{"type": "json_schema", "name": "search_request", "strict": true, "schema": schema}}}
	data, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/responses", bytes.NewReader(data))
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ParsedRequest{}, fmt.Errorf("не удалось обработать запрос автоматически")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ParsedRequest{}, fmt.Errorf("не удалось обработать запрос автоматически")
	}
	body, _ := io.ReadAll(resp.Body)
	parsed, err := parseAIResponse(body)
	if err != nil {
		return ParsedRequest{}, err
	}
	if err := validateParsedRequest(parsed, catalog); err != nil {
		return ParsedRequest{}, err
	}
	return parsed, nil
}

func mustJSON(value any) string { data, _ := json.Marshal(value); return string(data) }

func transcribeAudio(ctx context.Context, file multipart.File, header *multipart.FileHeader) (string, error) {
	key := os.Getenv("OPENAI_API_KEY")
	if key == "" {
		return "", fmt.Errorf("OPENAI_API_KEY не настроен; голосовой поиск недоступен")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", header.Filename)
	if err != nil {
		return "", err
	}
	if _, err = io.Copy(part, file); err != nil {
		return "", err
	}
	_ = writer.WriteField("model", transcribeModel())
	_ = writer.Close()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/audio/transcriptions", &body)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := (&http.Client{Timeout: 45 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("не удалось распознать голосовой запрос")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("не удалось распознать голосовой запрос")
	}
	var result struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || strings.TrimSpace(result.Text) == "" {
		return "", fmt.Errorf("голосовой запрос оказался пустым")
	}
	return result.Text, nil
}
