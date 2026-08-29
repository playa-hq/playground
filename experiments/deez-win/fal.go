package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Fal is a thin client over fal.ai's synchronous endpoints. It provides
// optional topic covers and illustrations for already-grounded questions.
// It never participates in deciding question text, options, answers, facts,
// or citations.
type Fal struct {
	apiKey string
	base   string
	client *http.Client
}

func NewFal(apiKey string) *Fal {
	return &Fal{apiKey: apiKey, base: "https://fal.run/", client: &http.Client{Timeout: 90 * time.Second}}
}

func (f *Fal) Enabled() bool { return f != nil && f.apiKey != "" }

const (
	falImageModel = "fal-ai/flux/schnell"
	falCutModel   = "fal-ai/birefnet"
	falQuizModel  = "fal-ai/flux-2-pro"
)

func (f *Fal) run(ctx context.Context, model string, in any, out any) error {
	raw, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.base+model, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Key "+f.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var e struct {
			Detail any `json:"detail"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return fmt.Errorf("fal %s: %s %v", model, resp.Status, e.Detail)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type fetchedImage struct {
	data        []byte
	contentType string
}

// Fetch downloads and checks a result so the app can serve it itself: the
// site's CSP is img-src 'self', and fal's CDN URLs are not forever.
func (f *Fal) Fetch(ctx context.Context, url string) (fetchedImage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fetchedImage{}, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return fetchedImage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fetchedImage{}, fmt.Errorf("fal fetch: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil {
		return fetchedImage{}, err
	}
	if len(data) > 8<<20 {
		return fetchedImage{}, errors.New("fal fetch: image exceeds 8 MiB")
	}
	contentType := http.DetectContentType(data)
	if !strings.HasPrefix(contentType, "image/") {
		return fetchedImage{}, fmt.Errorf("fal fetch: expected image, got %s", contentType)
	}
	return fetchedImage{data: data, contentType: contentType}, nil
}

// Illustrate renders the objects and returns the URL of a transparent PNG.
// Callers pass objects, never a topic string: see coverObjects.
func (f *Fal) Illustrate(ctx context.Context, objects []string) (string, error) {
	prompt := coverPrompt(objects)

	var gen struct {
		Images []struct {
			URL string `json:"url"`
		} `json:"images"`
	}
	err := f.run(ctx, falImageModel, map[string]any{
		"prompt": prompt, "image_size": "square", "num_inference_steps": 4,
		"num_images": 1, "output_format": "png",
	}, &gen)
	if err != nil {
		return "", err
	}
	if len(gen.Images) == 0 {
		return "", fmt.Errorf("fal: no image for %v", objects)
	}

	var cut struct {
		Image struct {
			URL string `json:"url"`
		} `json:"image"`
	}
	err = f.run(ctx, falCutModel, map[string]any{
		"image_url": gen.Images[0].URL, "output_format": "png", "refine_foreground": true,
	}, &cut)
	if err != nil {
		// A picture with a white ground still beats no picture.
		return gen.Images[0].URL, nil
	}
	return cut.Image.URL, nil
}

func (f *Fal) generateQuestionImage(ctx context.Context, topic string, q *Question) (string, error) {
	var out struct {
		Images []struct {
			URL string `json:"url"`
		} `json:"images"`
	}
	err := f.run(ctx, falQuizModel, map[string]any{
		"prompt":                neutralImagePrompt(topic, q),
		"image_size":            "landscape_4_3",
		"num_images":            1,
		"enable_safety_checker": true,
		"output_format":         "jpeg",
	}, &out)
	if err != nil {
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
	entities := strings.Join(q.Options, " and ")
	if strings.TrimSpace(entities) == "" {
		entities = "the possible answers"
	}

	return "A playful editorial illustration providing neutral atmosphere for a multiplayer trivia question. " +
		"General topic: " + strings.TrimSpace(topic) + ". General theme: " + neutralQuestionTheme(q) + ". " +
		"Represent both entities, " + entities + ", side by side with exactly equal visual prominence, scale, framing, lighting, detail, color intensity, and space. " +
		"Use neutral, characteristic visual atmosphere for each entity without rendering either entity's name. " +
		"Do not use the measured axis to compare the entities and do not imply that either one is higher, lower, larger, smaller, better, worse, winning, or correct. Never depict or infer the underlying values or answer. " +
		"Do not depict rankings, comparison results, quantities, size cues, maps, flags, charts, trophies, arrows, check marks, highlighting, or logos. " +
		"Absolutely no text, letters, words, numbers, captions, labels, signs, logos, or watermarks."
}

// neutralQuestionTheme removes entity-specific and comparative wording while
// retaining the general axis that gives the illustration useful context.
func neutralQuestionTheme(q *Question) string {
	theme := strings.TrimSpace(q.Prompt)
	if _, axis, ok := strings.Cut(theme, " — "); ok {
		theme = strings.TrimSpace(axis)
	}

	lower := strings.ToLower(strings.TrimSpace(theme))
	switch lower {
	case "which launched first?":
		return "launch history"
	case "which was founded first?":
		return "founding history"
	case "who was born first?":
		return "biographical history"
	}
	for _, prefix := range []string{"which has the higher ", "which has the earlier "} {
		if strings.HasPrefix(lower, prefix) {
			lower = strings.TrimPrefix(lower, prefix)
			break
		}
	}
	if theme = strings.TrimSpace(strings.TrimSuffix(lower, "?")); theme == "" {
		return "general knowledge"
	}
	return theme
}
