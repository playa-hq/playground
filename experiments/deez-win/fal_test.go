package main

import (
	"bytes"
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
	url, err := f.Illustrate(context.Background(), coverObjects("Big Tech (Apple, Microsoft)", nil))
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://cdn/cut.png" {
		t.Errorf("want the cut-out url, got %s", url)
	}
	if gotCutURL != "https://cdn/gen.png" {
		t.Errorf("cut-out should take the generated image, got %s", gotCutURL)
	}
	if strings.Contains(gotPrompt, "Apple") || strings.Contains(gotPrompt, "Big Tech") {
		t.Errorf("topic text or entity names must never reach the prompt: %s", gotPrompt)
	}
	if !strings.Contains(gotPrompt, "a microchip") || !strings.Contains(gotPrompt, "white background") {
		t.Errorf("prompt lost the objects or the flat ground: %s", gotPrompt)
	}
}

func TestCoverObjects(t *testing.T) {
	cases := []struct {
		topic      string
		industries []string
		want       string
	}{
		{"Spanish fintech startups", nil, "a leather wallet"},
		{"Big Tech companies", nil, "a microchip"},
		{"European banks", []string{"FINANCIAL_INSURANCE"}, "a leather wallet"},
		{"random things", nil, "a glass office tower"},
		{"car makers", nil, "a sports car"},
	}
	for _, c := range cases {
		got := coverObjects(c.topic, c.industries)
		if len(got) == 0 || got[0] != c.want {
			t.Errorf("coverObjects(%q, %v) = %v, want first %q", c.topic, c.industries, got, c.want)
		}
		for _, o := range got {
			if strings.Contains(strings.ToLower(c.topic), o) {
				t.Errorf("object %q echoes the topic", o)
			}
		}
	}
}

func TestQuestionImagesAreDownloadedAndServedSameOrigin(t *testing.T) {
	jpeg := []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00}
	var upstream *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("POST /fal-ai/flux-2-pro", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"images": []map[string]any{{"url": upstream.URL + "/cdn/question.jpg"}},
		})
	})
	mux.HandleFunc("GET /cdn/question.jpg", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(jpeg)
	})
	upstream = httptest.NewTLSServer(mux)
	defer upstream.Close()

	fal := &Fal{apiKey: "k", base: upstream.URL + "/", client: upstream.Client()}
	s := &Server{fal: fal}
	questions := []*Question{{Prompt: "Which launched first?", Options: []string{"A", "B"}}}
	s.addQuestionImages(context.Background(), "Space companies", questions, 1)

	imageURL := questions[0].ImageURL
	if !strings.HasPrefix(imageURL, "/question-images/") {
		t.Fatalf("question image URL = %q, want same-origin path", imageURL)
	}
	if strings.Contains(imageURL, upstream.URL) {
		t.Fatalf("question image leaks upstream CDN URL: %q", imageURL)
	}

	recorder := httptest.NewRecorder()
	s.handleGeneratedImage(recorder, httptest.NewRequest(http.MethodGet, imageURL, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET %s = %d", imageURL, recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("Content-Type = %q, want image/jpeg", got)
	}
	if !bytes.Equal(recorder.Body.Bytes(), jpeg) {
		t.Fatal("served question image differs from downloaded bytes")
	}
}

func TestFetchRejectsNonImageResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not an image"))
	}))
	defer upstream.Close()

	fal := &Fal{client: upstream.Client()}
	if _, err := fal.Fetch(context.Background(), upstream.URL); err == nil {
		t.Fatal("Fetch accepted a non-image response")
	}
}
