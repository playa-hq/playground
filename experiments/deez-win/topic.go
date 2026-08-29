package main

import (
	"context"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

// A TopicGraph is what Cala knows about one topic: the entities the topic
// names and, per entity, the axes that exist for it. Built once when the
// topic is picked, then read twice — to offer sub-topics, and to fetch values.
type TopicGraph struct {
	Entities []CalaEntity
	Intro    map[string]*CalaIntrospection // entity id → axes
}

const (
	graphEntityCap = 8 // entities kept per topic; more calls than this feels slow
	axisMinShared  = 3 // an axis needs this many entities to make a question
)

// resolveTopic turns free text into entities. A topic is usually a set
// ("Spanish fintechs"), which the query endpoint resolves; when it is one
// thing ("Apple") the query may name nothing, so fall back to fuzzy search and
// take the neighbours it returns.
func (c *Cala) resolveTopic(ctx context.Context, topic string) ([]CalaEntity, error) {
	rows, entities, err := c.Query(ctx, topic)
	if err != nil {
		return nil, err
	}
	entities = subjectsOnly(rows, entities)
	if len(entities) < axisMinShared {
		more, err := c.SearchEntities(ctx, topic, graphEntityCap)
		if err != nil && len(entities) == 0 {
			return nil, err
		}
		entities = mergeEntities(entities, more)
	}
	if len(entities) > graphEntityCap {
		entities = entities[:graphEntityCap]
	}
	return entities, nil
}

// subjectsOnly keeps the entities the result rows are *about*. The query
// also links every place, investor and country a row mentions — "Spanish
// fintechs" comes back with Barcelona, Y Combinator and Spain attached —
// and none of those belong in a round about fintechs. A row's subject is its
// first text column; an entity stays if one of its mentions matches one.
func subjectsOnly(rows []map[string]any, entities []CalaEntity) []CalaEntity {
	var subjects []string
	for _, row := range rows {
		for _, k := range sortedKeysStable(row) {
			if v, ok := row[k].(string); ok && v != "" {
				subjects = append(subjects, strings.ToLower(v))
				break
			}
		}
	}
	if len(subjects) == 0 {
		return entities
	}
	matches := func(e CalaEntity) bool {
		names := append([]string{e.Name}, e.Mentions...)
		for _, n := range names {
			n = strings.ToLower(n)
			for _, s := range subjects {
				if n == s || strings.HasPrefix(s, n) || strings.HasPrefix(n, s) {
					return true
				}
			}
		}
		return false
	}
	var out []CalaEntity
	for _, e := range entities {
		if matches(e) {
			out = append(out, e)
		}
	}
	if len(out) == 0 {
		return entities
	}
	return out
}

// sortedKeysStable puts a row's columns in the order the API sent them as far
// as Go's map allows: the subject column is conventionally first and named
// like the topic, so prefer keys that are not obviously attributes.
func sortedKeysStable(row map[string]any) []string {
	keys := sortedKeys(row)
	sort.SliceStable(keys, func(i, j int) bool {
		return subjectKeyRank(keys[i]) < subjectKeyRank(keys[j])
	})
	return keys
}

func subjectKeyRank(k string) int {
	k = strings.ToLower(k)
	switch {
	case k == "name" || k == "company" || k == "startup" || k == "entity" || k == "organization" || k == "bank":
		return 0
	case strings.Contains(k, "name") || strings.Contains(k, "compan"):
		return 1
	}
	return 2
}

func mergeEntities(a, b []CalaEntity) []CalaEntity {
	seen := map[string]bool{}
	var out []CalaEntity
	for _, e := range append(a, b...) {
		if e.ID == "" || seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		out = append(out, e)
	}
	return out
}

// BuildTopicGraph resolves the topic and introspects every entity in parallel.
// Results are cached by topic for the life of the process: resolution is the
// slow, model-backed step, and the same demo topics get picked all evening.
func (c *Cala) BuildTopicGraph(ctx context.Context, topic string) (*TopicGraph, error) {
	key := strings.ToLower(strings.TrimSpace(topic))
	if g, ok := c.graphs.Load(key); ok {
		return g.(*TopicGraph), nil
	}
	g, err := c.buildTopicGraph(ctx, topic)
	if err == nil {
		c.graphs.Store(key, g)
	}
	return g, err
}

// Warm resolves topics ahead of time so the suggestions on the pick screen
// answer instantly. Runs in the background; failures only cost a log line.
// Sequential on purpose: concurrent queries hit the rate limit.
func (c *Cala) Warm(topics []string) {
	go func() {
		for _, t := range topics {
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
			start := time.Now()
			g, err := c.BuildTopicGraph(ctx, t)
			cancel()
			if err != nil {
				log.Printf("cala: warm %q: %v", t, err)
				continue
			}
			log.Printf("cala: warm %q → %d entities, %d axes in %s", t, len(g.Entities), len(g.SubTopics(5)), time.Since(start).Round(time.Second))
		}
	}()
}

func (c *Cala) buildTopicGraph(ctx context.Context, topic string) (*TopicGraph, error) {
	entities, err := c.resolveTopic(ctx, topic)
	if err != nil {
		return nil, err
	}
	g := &TopicGraph{Entities: entities, Intro: map[string]*CalaIntrospection{}}

	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, e := range entities {
		wg.Add(1)
		go func(e CalaEntity) {
			defer wg.Done()
			in, err := c.Introspect(ctx, e.ID)
			if err != nil {
				return
			}
			mu.Lock()
			g.Intro[e.ID] = in
			mu.Unlock()
		}(e)
	}
	wg.Wait()
	return g, nil
}

// SubTopics ranks the axes shared across the topic's entities and returns the
// n most playable. Shared matters more than anything: an axis only one entity
// has cannot become a "which of these" question.
func (g *TopicGraph) SubTopics(n int) []SubTopic {
	type tally struct {
		st    SubTopic
		count int
	}
	counts := map[string]*tally{}
	bump := func(st SubTopic) {
		id := string(st.Kind) + ":" + st.Key
		if t, ok := counts[id]; ok {
			t.count++
			return
		}
		counts[id] = &tally{st: st, count: 1}
	}

	for _, in := range g.Intro {
		for _, p := range in.Properties {
			if isBoringProperty(p) {
				continue
			}
			bump(SubTopic{Key: p, Kind: propertyKind(p), Label: humanize(p)})
		}
		for _, r := range in.Relationships.Outgoing {
			if isBoringRelation(r) {
				continue
			}
			bump(SubTopic{Key: r, Kind: SubTopicRelation, Label: relationLabel(r)})
		}
		for _, metrics := range in.NumericalObservations {
			for _, m := range metrics {
				if isBoringMetric(m.Name) {
					continue
				}
				bump(SubTopic{Key: m.Name, Kind: SubTopicMetric, Label: metricLabel(m.Name)})
			}
		}
	}

	var out []tally
	for _, t := range counts {
		if t.count >= axisMinShared {
			out = append(out, *t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		si, sj := axisScore(out[i].st), axisScore(out[j].st)
		if si != sj {
			return si > sj
		}
		if out[i].count != out[j].count {
			return out[i].count > out[j].count
		}
		return out[i].st.Key < out[j].st.Key
	})

	// Metrics are plentiful and near-duplicates of each other (three flavours
	// of revenue), so they get at most two slots and one per family.
	res := make([]SubTopic, 0, n)
	metrics := 0
	families := map[string]bool{}
	for _, t := range out {
		if len(res) == n {
			break
		}
		if t.st.Kind == SubTopicMetric {
			fam := metricFamily(t.st.Key)
			if metrics >= 2 || families[fam] {
				continue
			}
			metrics++
			families[fam] = true
		}
		res = append(res, t.st)
	}
	return res
}

// MetricIDs returns, for one entity, the observation ids behind a metric
// name, grouped by type the way GetEntity wants them.
func (g *TopicGraph) MetricIDs(entityID string, names []string) map[string][]string {
	in := g.Intro[entityID]
	if in == nil {
		return nil
	}
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	out := map[string][]string{}
	for typ, metrics := range in.NumericalObservations {
		for _, m := range metrics {
			if want[m.Name] {
				out[typ] = append(out[typ], m.ID)
			}
		}
	}
	return out
}

/* ------------------------------------------------------------ ranking ----- */

// axisScore encodes what makes a good quiz axis. Numbers everyone has an
// intuition for beat accounting line items; a recognisable relationship beats
// a free-text address.
func axisScore(st SubTopic) int {
	k := strings.ToLower(st.Key)
	switch st.Kind {
	case SubTopicNumeric:
		switch {
		case strings.Contains(k, "employee"), strings.Contains(k, "population"):
			return 100
		case strings.Contains(k, "found"), strings.Contains(k, "incorporat"), strings.Contains(k, "birth"):
			return 95
		}
		return 80
	case SubTopicMetric:
		switch {
		case strings.HasPrefix(k, "revenue"):
			if k == "revenues" {
				return 91 // the plain series over its long-named siblings
			}
			return 90
		case strings.Contains(k, "net income"):
			return 85
		case strings.Contains(k, "total assets"), strings.Contains(k, "cash and cash"):
			return 60
		}
		return 40
	case SubTopicRelation:
		switch {
		case strings.Contains(k, "headquarters"), strings.Contains(k, "industry"), strings.Contains(k, "registered_in"):
			return 70
		case strings.Contains(k, "ceo"), strings.Contains(k, "founder"):
			return 65
		}
		return 30
	default:
		switch {
		case strings.Contains(k, "headquarters"), strings.Contains(k, "industry"), strings.Contains(k, "country"):
			return 55
		}
		return 20
	}
}

// propertyKind guesses from the key whether a property is numeric. The value
// itself decides at build time; this only affects ranking and the icon.
func propertyKind(key string) SubTopicKind {
	k := strings.ToLower(key)
	for _, hint := range []string{"count", "date", "year", "population", "amount", "number", "size"} {
		if strings.Contains(k, hint) {
			return SubTopicNumeric
		}
	}
	return SubTopicProperty
}

func isBoringProperty(name string) bool {
	switch strings.ToLower(name) {
	case "id", "uuid", "name", "slug", "url", "created_at", "updated_at", "entity_type",
		"aliases", "lei", "cik", "bics", "legal_name", "esg_policy", "description":
		return true
	}
	return false
}

// isBoringMetric drops series nobody should be quizzed on.
func isBoringMetric(name string) bool {
	k := strings.ToLower(name)
	return strings.Contains(k, "deprecated") || strings.Contains(k, "per share") ||
		strings.Contains(k, "weighted") || strings.Contains(k, "tax") && !strings.HasPrefix(k, "revenue")
}

// metricFamily groups "Revenues" and "Revenue from Contract with Customer…"
// so a round does not ask the same thing twice.
func metricFamily(name string) string {
	k := strings.ToLower(name)
	switch {
	case strings.HasPrefix(k, "revenue"), strings.HasPrefix(k, "sales"):
		return "revenue"
	case strings.Contains(k, "net income"):
		return "net income"
	case strings.Contains(k, "assets"):
		return "assets"
	case strings.Contains(k, "cash"):
		return "cash"
	}
	return k
}

// metricLabel shortens taxonomy names to what a host would say.
func metricLabel(name string) string {
	switch metricFamily(name) {
	case "revenue":
		return "Revenue"
	case "net income":
		return "Net income"
	}
	if i := strings.IndexAny(name, ",("); i > 0 {
		return strings.TrimSpace(name[:i])
	}
	return name
}

func isBoringRelation(name string) bool {
	k := strings.ToLower(name)
	return strings.Contains(k, "parent") || strings.Contains(k, "beneficiary") ||
		strings.Contains(k, "corporate_event") || strings.Contains(k, "subsidiary")
}

// relationLabel turns HAS_HEADQUARTERS_IN into "Headquarters".
func relationLabel(rel string) string {
	k := strings.ToLower(rel)
	for _, p := range []string{"has_", "is_", "operates_in_", "participates_in_"} {
		k = strings.TrimPrefix(k, p)
	}
	for _, s := range []string{"_of", "_in"} {
		k = strings.TrimSuffix(k, s)
	}
	return humanize(k)
}

func humanize(key string) string {
	s := strings.NewReplacer("_", " ", ".", " ", "-", " ").Replace(key)
	s = strings.TrimSpace(s)
	if s == "" {
		return key
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
