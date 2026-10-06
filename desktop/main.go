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
	adoptShellEnv()
	app := NewApp(change.Store{Home: workspace.Home()}, workspace.Roots())
	app.settings = loadSettings()
	bg, appearance := windowLook(app.settings)
	win, _ := loadWindow()
	err := wails.Run(&options.App{
		Title:            "Tandem",
		Width:            win.Width,
		Height:           win.Height,
		MinWidth:         minWidth,
		MinHeight:        minHeight,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: bg,
		OnStartup:        app.startup,
		StartHidden:      true,
		OnDomReady:       app.restoreWindow,
		OnBeforeClose:    app.rememberWindow,
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
