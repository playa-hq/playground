package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The documented shapes of flux/schnell and birefnet, served locally.
func TestIllustrateChainsGenerationAndCutout(t *testing.T) {
	var gotPrompt, gotCutURL string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /fal-ai/flux/schnell", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Key k" {
			http.Error(w, "no key", 401)
			return
		}
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		gotPrompt, _ = in["prompt"].(string)
		json.NewEncoder(w).Encode(map[string]any{"images": []map[string]any{{"url": "https://cdn/gen.png", "width": 1024, "height": 1024}}})
	})
	mux.HandleFunc("POST /fal-ai/birefnet", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		gotCutURL, _ = in["image_url"].(string)
		json.NewEncoder(w).Encode(map[string]any{"image": map[string]any{"url": "https://cdn/cut.png", "content_type": "image/png"}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	f := &Fal{apiKey: "k", client: srv.Client()}
	f.base = srv.URL + "/"
	url, err := f.Illustrate(context.Background(), "Big Tech (Apple, Microsoft)")
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://cdn/cut.png" {
		t.Errorf("want the cut-out url, got %s", url)
	}
	if gotCutURL != "https://cdn/gen.png" {
		t.Errorf("cut-out should take the generated image, got %s", gotCutURL)
	}
	if !strings.Contains(gotPrompt, "Apple") || !strings.Contains(gotPrompt, "white background") {
		t.Errorf("prompt lost the subject or the flat ground: %s", gotPrompt)
	}
}
