package main

import "testing"

func TestWindowRoundTrip(t *testing.T) {
	t.Setenv("TANDEM_CONFIG_DIR", t.TempDir())
	if s, ok := loadWindow(); ok || s.Width != defaultWidth || s.Height != defaultHeight {
		t.Fatalf("nothing saved: %+v %v, want the default size", s, ok)
	}
	want := windowState{X: 40, Y: 25, Width: 1500, Height: 950, Maximised: true}
	if err := saveWindow(want); err != nil {
		t.Fatal(err)
	}
	if got, ok := loadWindow(); !ok || got != want {
		t.Errorf("loaded %+v %v, want %+v", got, ok, want)
	}
	if err := saveWindow(windowState{Width: 300, Height: 200}); err != nil {
		t.Fatal(err)
	}
	if got, _ := loadWindow(); got.Width != minWidth || got.Height != minHeight {
		t.Errorf("undersized window loaded as %+v, want the minimum", got)
	}
}
