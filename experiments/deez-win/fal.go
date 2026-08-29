package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// FAL adds optional illustrations to already-grounded questions. It never
// participates in deciding the prompt, options, answer, fact, or citation.
type FAL struct {
	apiKey string
	client *http.Client
}

func NewFAL(apiKey string) *FAL {
	return &FAL{
		apiKey: apiKey,
		client: &http.Client{Timeout: 25 * time.Second},
	}
}

func (f *FAL) Enabled() bool { return f != nil && f.apiKey != "" }

// AddQuestionImages generates a small, bounded batch. Each question is
// independent and failures are deliberately non-fatal.
func (f *FAL) AddQuestionImages(ctx context.Context, topic string, questions []*Question, limit int) {
	if !f.Enabled() || limit <= 0 {
		return
	}
	if limit > len(questions) {
		limit = len(questions)
	}

	var wg sync.WaitGroup
	for i := 0; i < limit; i++ {
		q := questions[i]
		wg.Add(1)
		go func(index int, question *Question) {
			defer wg.Done()
			imageURL, err := f.generateQuestionImage(ctx, topic, question)
			if err != nil {
				log.Printf("fal: question %d image skipped: %v", index+1, err)
				return
			}
			question.ImageURL = imageURL
		}(i, q)
	}
	wg.Wait()
}

func (f *FAL) generateQuestionImage(ctx context.Context, topic string, q *Question) (string, error) {
	body := map[string]any{
		"prompt":                neutralImagePrompt(topic, q),
		"image_size":            "landscape_4_3",
		"num_images":            1,
		"enable_safety_checker": true,
		"output_format":         "jpeg",
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://fal.run/fal-ai/flux/schnell", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Key "+f.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("request returned %s", resp.Status)
	}

	var out struct {
		Images []struct {
			URL string `json:"url"`
		} `json:"images"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Images) == 0 || out.Images[0].URL == "" {
		return "", errors.New("response contained no image")
	}
	imageURL, err := url.Parse(out.Images[0].URL)
	if err != nil || imageURL.Scheme != "https" || imageURL.Host == "" {
		return "", errors.New("response contained an invalid image URL")
	}
	return imageURL.String(), nil
}

func neutralImagePrompt(topic string, q *Question) string {
	subject := strings.TrimSpace(topic)
	if q.Kind == "higher_lower" && len(q.Options) > 0 {
		subject = strings.Join(q.Options, " and ")
	} else if entity, _, ok := strings.Cut(q.Prompt, " — "); ok && strings.TrimSpace(entity) != "" {
		subject = strings.TrimSpace(entity)
	}

	return "A playful editorial illustration for a multiplayer trivia game about " + subject + ". " +
		"Decorative atmosphere only: do not communicate or hint at the correct answer. " +
		"Give every depicted subject exactly equal size, lighting, detail, and visual prominence in a balanced neutral composition. " +
		"Do not depict rankings, comparison results, quantities, scale differences, relationships, maps, flags, charts, trophies, arrows, check marks, or highlighting. " +
		"Absolutely no text, letters, words, numbers, captions, labels, signs, logos, or watermarks."
}
