// Copyright (C) 2023-2026 Òscar Casajuana Alonso

// Command manga-downloader-gui is the desktop front end of the CLI. It lives
// in its own module so the webview's cgo and its platform libraries never
// reach the CLI's module; see the gui skill.
package main

import (
	"context"
	"embed"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/elboletaire/manga-downloader/browser"
	mdhttp "github.com/elboletaire/manga-downloader/http"
	"github.com/elboletaire/manga-downloader/notice"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

// Version and Tag are set at link time by the release workflow (-X), the same
// way the CLI gets its version: Version is the short commit sha, Tag the
// release tag. Both stay at their defaults in a plain build.
var (
	Version = "dev"
	Tag     = ""
)

// assets is the built frontend. It's produced by `pnpm build` in frontend/,
// which make build/gui and the release workflow run before compiling.
//
//go:embed all:frontend/dist
var assets embed.FS

// appIcon is the window icon on Linux. It's the free-shape mark rather than
// the macOS tile, which would look like a box on a Linux taskbar. Windows and
// macOS take theirs from build/ at packaging time.
//
//go:embed build/linux/icon.png
var appIcon []byte

func main() {
	rt := &wailsRuntime{}
	var app *App
	// notify is called from the download goroutines too, so it reads app
	// through the pointer set before the window opens. Warnings raised while
	// the options load come before that, hence the nil check.
	// a load asks a site for its series page several times (identifying
	// it, then its title and chapters): within a load, the page is fetched
	// once. Each load starts by forgetting the last one's pages.
	mdhttp.PageMemory = 30 * time.Second
	ctrl, prefsPath := newSession(func() {
		if app != nil {
			app.notify()
		}
	})
	ctrl.SetLibrary(openLibrary())
	covers := coversDir()
	if covers != "" {
		ctrl.SetCovers(newCoverStore(covers))
	}
	if lists := dataDir("lists"); lists != "" {
		ctrl.SetLists(newListStore(lists))
	}
	app = newApp(ctrl, rt, prefsPath, Tag, versionString())

	err := wails.Run(&options.App{
		Title:            "Manga Downloader",
		Width:            1100,
		Height:           760,
		MinWidth:         900,
		MinHeight:        600,
		BackgroundColour: startupBackground(),
		AssetServer:      &assetserver.Options{Assets: assets, Handler: coverHandler(covers)},
		OnStartup: func(ctx context.Context) {
			app.startup(ctx, rt)
		},
		OnShutdown: app.shutdown,
		Bind:       []interface{}{app},
		Linux:      &linux.Options{Icon: appIcon},
		// without Mac options Wails disables the zoom button. The title bar
		// is hidden: the app's header is the window's top edge, and the
		// window buttons stay where macOS puts them (see frame.ts)
		Mac: &mac.Options{TitleBar: mac.TitleBarHidden()},
	})
	// OnShutdown already closed it; this covers a Run that failed or returned
	// without calling it
	browser.Close()
	if err != nil {
		log.Fatal(err)
	}
}

// newSession returns the controller with the saved options applied, and the
// path they're saved at (empty when there is none). The controller is created
// first, so the warnings loading raises reach its log instead of stdout.
func newSession(notify func()) (*Controller, string) {
	settings := defaultSettings()
	ctrl := NewController(settings, notify)

	prefsPath, err := guiConfigPath()
	if err != nil {
		// the window still works, the options just won't outlive this run
		notice.Warn("%v", err)
		return ctrl, ""
	}
	if err := loadOptions(prefsPath, settings); err != nil {
		notice.Warn("ignoring the saved options: %v", err)
	}

	return ctrl, prefsPath
}

// openLibrary loads library.json from the config dir, next to gui.json. A
// file that doesn't parse is moved aside and the library starts empty, with
// a warning; without a config dir the library lasts for this run only.
func openLibrary() *Library {
	dir, err := os.UserConfigDir()
	if err != nil {
		notice.Warn("no config dir for the library: %v", err)
		return NewLibrary("")
	}
	lib := NewLibrary(filepath.Join(dir, "manga-downloader", "library.json"))
	if err := lib.Load(); err != nil {
		notice.Warn("ignoring the saved library: %v", err)
	}
	return lib
}

// coversDir is where the followed series' covers are kept, next to
// library.json; "" without a config dir (then no cover is fetched)
func coversDir() string {
	return dataDir("covers")
}

// dataDir is the folder of one kind of the app's data (covers, lists) next
// to library.json; "" without a config dir
func dataDir(name string) string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "manga-downloader", name)
}

// versionString is what the window's About text shows: the tag when there is
// one, the build's version otherwise
func versionString() string {
	if Tag != "" {
		return Tag
	}

	return Version
}
