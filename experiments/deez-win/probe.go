package main

import (
	"context"
	"fmt"
	"os"
	"time"
)

// runProbe is the coverage check behind the first kill criterion: type a
// topic, see which entities and axes Cala gives back and what the questions
// would be. Run it before trusting a topic on stage.
func runProbe(c *Cala, topic string) int {
	if !c.Enabled() {
		fmt.Fprintln(os.Stderr, "CALA_API_KEY is not set")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	start := time.Now()
	g, err := c.BuildTopicGraph(ctx, topic)
	if err != nil {
		fmt.Fprintln(os.Stderr, "resolve:", err)
		return 1
	}
	fmt.Printf("topic %q → %d entities (%s)\n", topic, len(g.Entities), time.Since(start).Round(time.Millisecond))
	for _, e := range g.Entities {
		in := g.Intro[e.ID]
		axes := "no introspection"
		if in != nil {
			n := 0
			for _, m := range in.NumericalObservations {
				n += len(m)
			}
			axes = fmt.Sprintf("%d props, %d rels, %d metrics", len(in.Properties), len(in.Relationships.Outgoing), n)
		}
		fmt.Printf("  %-32s %-12s %s\n", e.Name, e.EntityType, axes)
	}

	subs := g.SubTopics(5)
	fmt.Printf("\nsub-topics offered:\n")
	for _, st := range subs {
		fmt.Printf("  [%s] %s\n", st.Kind, st.Label)
	}
	if len(subs) == 0 {
		fmt.Println("  none — this topic is not playable")
		return 1
	}

	for i := range subs {
		subs[i].ClaimedBy = "probe"
	}
	srv := &Server{cala: c}
	details := srv.fetchDetails(ctx, g, subs)
	fmt.Printf("\nquestions (%d entities answered, %s total):\n", len(details), time.Since(start).Round(time.Millisecond))
	for _, st := range subs {
		for _, q := range axisQuestions(st, details, 2) {
			fmt.Printf("  %s\n    %v → %s\n    %s  [%s]\n", q.Prompt, q.Options, q.Options[q.Answer], q.Fact, q.Source)
		}
	}
	return 0
}
