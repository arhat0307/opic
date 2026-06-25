package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	service := NewAppService()
	app := application.New(application.Options{
		Name:        "OPIc Flow",
		Description: "AI voice practice for the OPIc speaking test",
		Services: []application.Service{
			application.NewService(service),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "OPIc Flow",
		Width:            1280,
		Height:           820,
		MinWidth:         980,
		MinHeight:        680,
		BackgroundColour: application.NewRGB(246, 247, 243),
		URL:              "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

