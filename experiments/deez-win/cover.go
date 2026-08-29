package main

import (
	"sort"
	"strings"
)

// coverObjects turns a topic into the things a sticker of it should show.
//
// The topic text itself must never reach the image prompt: FLUX typesets any
// phrase it is given ("Spanish fintech startups" came back as a sticker
// reading "Spaniich finartups"), and naming companies draws their logos. So
// the prompt only ever names objects. Cala's industry values, when the graph
// has them, are the best hint; the topic's own words fill in otherwise.
func coverObjects(topic string, industries []string) []string {
	hints := append([]string{strings.ToLower(topic)}, lowerAll(industries)...)
	text := strings.Join(hints, " ")

	var out []string
	add := func(objs ...string) {
		for _, o := range objs {
			if !contains(out, o) && len(out) < 3 {
				out = append(out, o)
			}
		}
	}
	// Order matters: the most specific families first, so "fintech" gives
	// cards and coins before "tech" gives a chip.
	for _, f := range objectFamilies {
		for _, k := range f.keys {
			if strings.Contains(text, k) {
				add(f.objects...)
				break
			}
		}
	}
	if len(out) == 0 {
		add("a glass office tower", "a briefcase", "a gold coin")
	}
	return out
}

type objectFamily struct {
	keys    []string
	objects []string
}

var objectFamilies = []objectFamily{
	{[]string{"fintech", "payment", "bank", "financ", "insur", "crypto", "invest", "wealth"}, []string{"a leather wallet", "a stack of gold coins", "a smartphone"}},
	{[]string{"space", "rocket", "satellite", "aerospace", "nasa"}, []string{"a rocket", "a satellite", "a planet with rings"}},
	{[]string{"car", "auto", "vehicle", "ev ", "electric"}, []string{"a sports car", "a charging plug", "a steering wheel"}},
	{[]string{"pharma", "biotech", "health", "medic", "drug"}, []string{"a pill capsule", "a dna helix", "a microscope"}},
	{[]string{"energy", "oil", "solar", "wind", "power", "utility"}, []string{"a wind turbine", "a solar panel", "a lightning bolt"}},
	{[]string{"retail", "shop", "fashion", "commerce", "store"}, []string{"a shopping bag", "a price tag", "a sneaker"}},
	{[]string{"food", "restaurant", "beverage", "drink", "coffee"}, []string{"a coffee cup", "a burger", "a fork and knife"}},
	{[]string{"airline", "travel", "hotel", "tourism"}, []string{"an airplane", "a suitcase", "a passport"}},
	{[]string{"game", "gaming", "esport"}, []string{"a game controller", "a joystick", "a trophy"}},
	{[]string{"ai ", "ai lab", "artificial", "machine learning", "robot"}, []string{"a friendly robot head", "a glowing neural network", "a microchip"}},
	{[]string{"tech", "software", "cloud", "internet", "semiconductor", "chip", "computer"}, []string{"a microchip", "a laptop", "a smartphone"}},
	{[]string{"startup", "unicorn", "founder"}, []string{"a rocket", "a lightbulb", "a unicorn"}},
	{[]string{"cit", "town", "capital"}, []string{"a city skyline", "a cathedral", "a tram"}},
	{[]string{"countr", "nation", "europe", "asia", "america"}, []string{"a globe", "a waving flag", "a compass"}},
	{[]string{"media", "film", "music", "stream"}, []string{"a film clapperboard", "a vinyl record", "a popcorn bucket"}},
}

// coverPrompt is the whole image prompt. Style words are fixed so every
// cover in a session looks like a set; the only variable is the objects.
func coverPrompt(objects []string) string {
	return "a single bold glossy die-cut sticker illustration, flat vector style with thick outlines, of " +
		joinObjects(objects) + " arranged together as one composition. centered, isolated on a flat solid white " +
		"background. absolutely no text, no letters, no numbers, no logos, no brand marks, no typography."
}

func joinObjects(objs []string) string {
	switch len(objs) {
	case 0:
		return "a gold coin"
	case 1:
		return objs[0]
	}
	return strings.Join(objs[:len(objs)-1], ", ") + " and " + objs[len(objs)-1]
}

func lowerAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, strings.ToLower(strings.ReplaceAll(s, "_", " ")))
	}
	sort.Strings(out)
	return out
}
