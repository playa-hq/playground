package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Cala is a thin client over the Cala knowledge API (https://docs.cala.ai).
//
// The game leans on four calls:
//   - POST /v1/knowledge/query             a topic ("Spanish fintechs") → the entities it names
//   - GET  /v1/entities?name=              fuzzy lookup when the topic is one thing, not a set
//   - GET  /v1/entities/{id}/introspection which properties, relationships and metrics exist
//   - POST /v1/entities/{id}               the values themselves, each with its sources
//
// Introspection is the important one: it tells us what is answerable *before*
// we ask, so the sub-topics offered to players are axes the graph really holds.
type Cala struct {
	apiKey  string
	baseURL string
	client  *http.Client
	graphs  sync.Map // topic → *TopicGraph
}

func NewCala(apiKey string) *Cala {
	return &Cala{
		apiKey:  apiKey,
		baseURL: envOr("CALA_URL", "https://api.cala.ai"),
		// knowledge/query is model-backed and takes 35–60s on a set; everything
		// else answers in about a second.
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

func (c *Cala) Enabled() bool { return c.apiKey != "" }

type CalaEntity struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	EntityType  string `json:"entity_type"`
	Description string `json:"description"`
	// Mentions is how the entity was named in query results — the join key
	// between a result row and the entity it is about.
	Mentions []string `json:"mentions,omitempty"`
}

// CalaMetric is one numerical observation series as listed by introspection.
type CalaMetric struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Unit string `json:"unit"`
}

type CalaIntrospection struct {
	Properties    []string `json:"properties"`
	Relationships struct {
		Outgoing []string `json:"outgoing"`
		Incoming []string `json:"incoming"`
	} `json:"relationships"`
	NumericalObservations map[string][]CalaMetric `json:"numerical_observations"`
}

// CalaSource is a citation flattened to what the UI shows.
type CalaSource struct {
	Name string
	URL  string
	Date string
}

// CalaValue is one grounded fact: a value and where it came from.
type CalaValue struct {
	Value  any
	Source CalaSource
}

// CalaEntityDetail is the subset of GetEntityResponse the game reads.
type CalaEntityDetail struct {
	ID         string
	Name       string
	Properties map[string]CalaValue // property key → value
	Relations  map[string]CalaValue // relationship type → first related entity's name
	Metrics    map[string]CalaValue // metric name → latest value (float64)
	Units      map[string]string    // metric name → unit
}

func (c *Cala) do(ctx context.Context, method, path string, body any, out any) error {
	// Limits are per minute and undocumented (measured: ~5 POST /entities
	// and one knowledge/query in flight). Back off in growing steps rather
	// than hammer; a whole minute of waiting still beats a round without data.
	var err error
	for attempt, wait := 0, 10*time.Second; attempt < 4; attempt, wait = attempt+1, wait+10*time.Second {
		err = c.once(ctx, method, path, body, out)
		if err == nil || !strings.Contains(err.Error(), "429") {
			return err
		}
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return err
		}
	}
	return err
}

func (c *Cala) once(ctx context.Context, method, path string, body any, out any) error {
	var reader = bytes.NewReader(nil)
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
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
		var e struct {
			Error struct{ Message string } `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return fmt.Errorf("cala %s %s: %s %s", method, path, resp.Status, e.Error.Message)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// SearchEntities is fuzzy lookup by name, most relevant first.
func (c *Cala) SearchEntities(ctx context.Context, name string, limit int) ([]CalaEntity, error) {
	if limit <= 0 {
		limit = 10
	}
	q := url.Values{"name": {name}, "limit": {strconv.Itoa(limit)}}
	var out struct {
		Entities []CalaEntity `json:"entities"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/entities?"+q.Encode(), nil, &out); err != nil {
		return nil, err
	}
	return out.Entities, nil
}

// Introspect reports which axes exist for an entity.
func (c *Cala) Introspect(ctx context.Context, entityID string) (*CalaIntrospection, error) {
	var out CalaIntrospection
	if err := c.do(ctx, http.MethodGet, "/v1/entities/"+url.PathEscape(entityID)+"/introspection", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Query runs natural language or dot-notation and returns the rows plus the
// entities Cala identified in them.
func (c *Cala) Query(ctx context.Context, input string) ([]map[string]any, []CalaEntity, error) {
	var out struct {
		Results  []map[string]any `json:"results"`
		Entities []CalaEntity     `json:"entities"`
	}
	body := map[string]any{"input": input, "return_entities": true}
	if err := c.do(ctx, http.MethodPost, "/v1/knowledge/query", body, &out); err != nil {
		return nil, nil, err
	}
	return out.Results, out.Entities, nil
}

// GetEntity fetches exactly the requested axes for one entity. metricIDs is
// keyed by observation type, as introspection reports them.
func (c *Cala) GetEntity(ctx context.Context, id string, props, rels []string, metricIDs map[string][]string) (*CalaEntityDetail, error) {
	body := map[string]any{}
	if len(props) > 0 {
		body["properties"] = props
	}
	if len(rels) > 0 {
		out := map[string]any{}
		for _, r := range rels {
			out[r] = map[string]any{"limit": 3}
		}
		body["relationships"] = map[string]any{"outgoing": out}
	}
	if len(metricIDs) > 0 {
		body["numerical_observations"] = metricIDs
	}

	var raw struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Properties map[string]struct {
			Value   any               `json:"value"`
			Sources []json.RawMessage `json:"sources"`
		} `json:"properties"`
		Relationships struct {
			Outgoing map[string][]struct {
				Name       string `json:"name"`
				Properties struct {
					Sources []json.RawMessage `json:"sources"`
				} `json:"properties"`
			} `json:"outgoing"`
		} `json:"relationships"`
		NumericalObservations []struct {
			Name       string `json:"name"`
			Properties struct {
				Unit string `json:"unit"`
			} `json:"properties"`
			Data []struct {
				Time   string  `json:"time"`
				Value  float64 `json:"value"`
				Origin struct {
					Name   string `json:"name"`
					Source string `json:"source"`
					URL    string `json:"url"`
				} `json:"origin"`
			} `json:"data"`
		} `json:"numerical_observations"`
	}
	if err := c.do(ctx, http.MethodPost, "/v1/entities/"+url.PathEscape(id), body, &raw); err != nil {
		return nil, err
	}

	d := &CalaEntityDetail{
		ID: raw.ID, Name: raw.Name,
		Properties: map[string]CalaValue{},
		Relations:  map[string]CalaValue{},
		Metrics:    map[string]CalaValue{},
		Units:      map[string]string{},
	}
	for k, p := range raw.Properties {
		if p.Value == nil {
			continue
		}
		d.Properties[k] = CalaValue{Value: p.Value, Source: firstSource(p.Sources)}
	}
	for rel, targets := range raw.Relationships.Outgoing {
		if len(targets) == 0 || targets[0].Name == "" {
			continue
		}
		d.Relations[rel] = CalaValue{Value: targets[0].Name, Source: firstSource(targets[0].Properties.Sources)}
	}
	for _, m := range raw.NumericalObservations {
		latest := -1
		for i, pt := range m.Data {
			if latest < 0 || pt.Time > m.Data[latest].Time {
				latest = i
			}
		}
		if latest < 0 {
			continue
		}
		pt := m.Data[latest]
		name := pt.Origin.Source
		if pt.Origin.Name != "" {
			name = strings.TrimSpace(pt.Origin.Source + " " + pt.Origin.Name)
		}
		d.Metrics[m.Name] = CalaValue{Value: pt.Value, Source: CalaSource{Name: name, URL: pt.Origin.URL, Date: pt.Time[:min(10, len(pt.Time))]}}
		d.Units[m.Name] = m.Properties.Unit
	}
	return d, nil
}

// firstSource flattens the first citation. Cala's "document" is a URL string
// on most properties and an object with an endpoint on a few (SEC full-text
// search); both are handled.
func firstSource(raw []json.RawMessage) CalaSource {
	if len(raw) == 0 {
		return CalaSource{}
	}
	var s struct {
		Name     string `json:"name"`
		Date     string `json:"date"`
		Document any    `json:"document"`
	}
	if err := json.Unmarshal(raw[0], &s); err != nil {
		return CalaSource{}
	}
	out := CalaSource{Name: s.Name, Date: s.Date}
	switch d := s.Document.(type) {
	case string:
		out.URL = d
	case map[string]any:
		if ep, ok := d["endpoint"].(string); ok {
			out.URL = ep
		}
	}
	if out.Name == "" && out.URL != "" {
		if u, err := url.Parse(out.URL); err == nil {
			out.Name = u.Host
		}
	}
	return out
}
