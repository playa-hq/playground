package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Cala is a thin client over the Cala knowledge API.
//
// The game leans on three primitives:
//   - /v1/entities                        resolve a player's typed topic to real entities
//   - /v1/entities/{id}/introspection     ask what is actually answerable about one
//   - /v1/knowledge/query                 pull typed values with source traceability
//
// Introspection is the important one: it returns the properties, relationships
// and numerical observations Cala holds for an entity, which is exactly the
// list of sub-topics a question can be built from. We never invent an axis and
// hope for data — we read the axes off the graph and build from those.
type Cala struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func NewCala(apiKey string) *Cala {
	return &Cala{
		apiKey:  apiKey,
		baseURL: "https://api.cala.ai",
		client:  &http.Client{Timeout: 25 * time.Second},
	}
}

func (c *Cala) Enabled() bool { return c.apiKey != "" }

type CalaEntity struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	EntityType  string `json:"entity_type"`
	Description string `json:"description"`
}

type CalaIntrospection struct {
	Properties            []string       `json:"properties"`
	NumericalObservations map[string]any `json:"numerical_observations"`
	Relationships         struct {
		Outgoing []string `json:"outgoing"`
		Incoming []string `json:"incoming"`
	} `json:"relationships"`
}

type CalaSource struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func (c *Cala) do(ctx context.Context, method, path string, body any, out any) error {
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-KEY", c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("cala %s %s: %s", method, path, resp.Status)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// SearchEntities resolves free text to candidate entities.
func (c *Cala) SearchEntities(ctx context.Context, query string, limit int) ([]CalaEntity, error) {
	if limit <= 0 {
		limit = 10
	}
	path := fmt.Sprintf("/v1/entities?query=%s&limit=%d", urlQueryEscape(query), limit)

	var out struct {
		Entities []CalaEntity `json:"entities"`
		Results  []CalaEntity `json:"results"`
	}
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	if len(out.Entities) > 0 {
		return out.Entities, nil
	}
	return out.Results, nil
}

// Introspect reports which axes exist for an entity.
func (c *Cala) Introspect(ctx context.Context, entityID string) (*CalaIntrospection, error) {
	var out CalaIntrospection
	path := "/v1/entities/" + urlPathEscape(entityID) + "/introspection"
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Query runs natural language or dot-notation (e.g. "OpenAI.founded.year").
func (c *Cala) Query(ctx context.Context, input string) ([]map[string]any, []CalaEntity, error) {
	body := map[string]any{"input": input, "return_entities": true}

	var out struct {
		Results  []map[string]any `json:"results"`
		Entities []CalaEntity     `json:"entities"`
	}
	if err := c.do(ctx, http.MethodPost, "/v1/knowledge/query", body, &out); err != nil {
		return nil, nil, err
	}
	return out.Results, out.Entities, nil
}

// SubTopics turns an entity's introspection into the five most playable axes.
//
// Numerical observations rank first: they yield higher/lower and closest-guess
// questions with an unambiguous right answer and no model in the loop.
// Relationships come next, then plain properties.
func SubTopics(in *CalaIntrospection, n int) []SubTopic {
	if in == nil {
		return nil
	}
	var out []SubTopic

	for name := range in.NumericalObservations {
		out = append(out, SubTopic{Key: name, Kind: SubTopicNumeric, Label: humanize(name)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })

	rels := append([]string{}, in.Relationships.Outgoing...)
	sort.Strings(rels)
	for _, name := range rels {
		out = append(out, SubTopic{Key: name, Kind: SubTopicRelation, Label: humanize(name)})
	}

	props := append([]string{}, in.Properties...)
	sort.Strings(props)
	for _, name := range props {
		if isBoringProperty(name) {
			continue
		}
		out = append(out, SubTopic{Key: name, Kind: SubTopicProperty, Label: humanize(name)})
	}

	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

// isBoringProperty drops plumbing fields that make dull questions.
func isBoringProperty(name string) bool {
	switch strings.ToLower(name) {
	case "id", "uuid", "name", "slug", "url", "created_at", "updated_at", "entity_type":
		return true
	}
	return false
}

func humanize(key string) string {
	s := strings.NewReplacer("_", " ", ".", " ", "-", " ").Replace(key)
	s = strings.TrimSpace(s)
	if s == "" {
		return key
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
