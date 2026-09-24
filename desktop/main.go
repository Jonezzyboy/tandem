package main

import (
	"embed"
	"log"

	"github.com/jonezzyboy/tandem/internal/change"
	"github.com/jonezzyboy/tandem/internal/workspace"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	adoptShellPath()
	app := NewApp(change.Store{Home: workspace.Home()}, workspace.Roots())
	app.settings = loadSettings()
	bg, appearance := windowLook(app.settings)
	err := wails.Run(&options.App{
		Title:            "Tandem",
		Width:            1320,
		Height:           860,
		MinWidth:         980,
		MinHeight:        620,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: bg,
		OnStartup:        app.startup,
		Bind:             []any{app},
		Mac: &mac.Options{
			TitleBar:   mac.TitleBarHiddenInset(),
			Appearance: appearance,
			About:      &mac.AboutInfo{Title: "Tandem", Message: "One change, many repos."},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
