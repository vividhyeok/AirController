package main

import "testing"

func TestSanitizeFeatureConfig(t *testing.T) {
	cfg := FeatureConfig{
		Favorites: []Favorite{
			{URL: "https://www.youtube.com", Label: "YouTube"},
			{URL: "https://www.youtube.com", Label: "duplicate"},
			{URL: "file:///C:/Windows", Label: "bad"},
		},
		RecentHistory: []string{
			"https://www.youtube.com/watch?v=1",
			"https://www.youtube.com/watch?v=1",
			"file:///C:/Windows",
		},
		MouseSensitivity:  4.5,
		ScrollSensitivity: 2.5,
		LastTab:           "apps",
	}

	got := sanitizeFeatureConfig(cfg)
	if len(got.Favorites) != 1 {
		t.Fatalf("favorites len = %d, want 1", len(got.Favorites))
	}
	if got.Favorites[0].Label != "YouTube" {
		t.Fatalf("favorite label = %q, want YouTube", got.Favorites[0].Label)
	}
	if len(got.RecentHistory) != 1 {
		t.Fatalf("recent history len = %d, want 1", len(got.RecentHistory))
	}
	if got.MouseSensitivity != 4.5 || got.ScrollSensitivity != 2.5 {
		t.Fatalf("sensitivities = %v/%v, want 4.5/2.5", got.MouseSensitivity, got.ScrollSensitivity)
	}
	if got.LastTab != "apps" {
		t.Fatalf("last tab = %q, want apps", got.LastTab)
	}
}

func TestSanitizeFeatureConfigFallsBackToDefaults(t *testing.T) {
	got := sanitizeFeatureConfig(FeatureConfig{
		MouseSensitivity:  99,
		ScrollSensitivity: -1,
		LastTab:           "invalid",
	})

	if got.MouseSensitivity != 2.5 {
		t.Fatalf("mouse sensitivity = %v, want 2.5", got.MouseSensitivity)
	}
	if got.ScrollSensitivity != 3 {
		t.Fatalf("scroll sensitivity = %v, want 3", got.ScrollSensitivity)
	}
	if got.LastTab != "touch" {
		t.Fatalf("last tab = %q, want touch", got.LastTab)
	}
}
