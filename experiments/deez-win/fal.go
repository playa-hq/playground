package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Fal is a thin client over fal.ai's synchronous endpoints.
//
// Two models make a topic illustration:
//   - fal-ai/flux/schnell  four-step FLUX, ~2s, cheap: the picture itself
//   - fal-ai/birefnet      segmentation: cuts the subject out onto transparency
//
// The prompt asks FLUX for objects on a flat white ground so BiRefNet has an
// easy edge to find; the result is an RGBA PNG that sits on any field.
type Fal struct {
	apiKey string
	base   string
	client *http.Client
}

func NewFal(apiKey string) *Fal {
	return &Fal{apiKey: apiKey, base: "https://fal.run/", client: &http.Client{Timeout: 90 * time.Second}}
}

func (f *Fal) Enabled() bool { return f.apiKey != "" }

const (
	falImageModel = "fal-ai/flux/schnell"
	falCutModel   = "fal-ai/birefnet"
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

// Fetch downloads a result so the app can serve it itself: the site's CSP
// is img-src 'self', and fal's CDN URLs are not forever.
func (f *Fal) Fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fal fetch: %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
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
