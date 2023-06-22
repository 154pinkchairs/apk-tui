package main

import (
	"log"
	"math"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type (
	PackageInfo struct {
		Name     string
		Provides []string
	}
)

func getPackageProvides(packageName string) []string {
	cmd := exec.Command("doas", "apk", "info", "--provides", packageName)
	output, _ := cmd.Output()

	return strings.Split(string(output), "\n")
}

func getPackages() ([]string, error) {
	cmd := exec.Command("doas", "apk", "search")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	packages := strings.Split(string(output), "\n")
	versionRegex := regexp.MustCompile(`-[0-9].*`)

	for i, p := range packages {
		packages[i] = versionRegex.ReplaceAllString(p, "")
	}

	return packages, nil
}

func worker(packages []string, packagesMap *sync.Map, wg *sync.WaitGroup) {
	for _, p := range packages {
		provides := getPackageProvides(p)
		packagesMap.Store(p, provides)
	}
	wg.Done()
}

func main() {
	packages, err := getPackages()
	if err != nil {
		log.Fatal(err)
	}

	app := tview.NewApplication()

	// Create a new tabbed view
	flex := tview.NewFlex().SetDirection(tview.FlexRow)
	tabs := tview.NewList().ShowSecondaryText(true)
	tabs.AddItem("Packages", "", 'p', nil)
	tabs.AddItem("Search", "", 's', nil)
	tabs.AddItem("Quit", "", 'q', func() {
		app.Stop()
	})
	tabs.AddItem("Provided files", "", 'f', func() {
		packagesMap := &sync.Map{}

		var wg sync.WaitGroup
		intermediate := math.Round(float64(runtime.NumCPU()) * 0.75)
		numWorkers := int(intermediate)
		packagesPerWorker := len(packages) / numWorkers
		for w := 0; w < numWorkers; w++ {
			start := w * packagesPerWorker
			end := start + packagesPerWorker
			if w == numWorkers-1 {
				end = len(packages) // Make sure to include any remaining packages
			}
			wg.Add(1)
			go worker(packages[start:end], packagesMap, &wg)
		}

		wg.Wait()
	})

	tabs.SetSelectedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		switch index {
		case 0:
			// Create a new new list
			list := tview.NewList()
			list.SetBorder(true).SetTitle("Packages")
		case 1:
			// Create a new input field
			inputField := tview.NewInputField()
			inputField.SetLabel("Search").SetFieldWidth(10).SetAcceptanceFunc(tview.InputFieldInteger)
			inputField.SetDoneFunc(func(key tcell.Key) {
				app.Stop()
			})
			inputField.SetBorder(true).SetTitle("Search")
			inputField.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
				switch event.Key() {
				case tcell.KeyEnter:
					app.Stop()
				}
				return event
			})
			app.SetRoot(inputField, true)
		case 2:
			app.Stop()
		case 3:
			// Create a new text view for the gutter
			gutter := tview.NewTextView()
			gutter.SetText("Navigation info")
			gutter.SetBorder(true).SetTitle("Provided files")
			app.SetRoot(gutter, true)
		}
	})

	tabs.SetChangedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
		switch index {
		case 0:
			// Create a new list
			list := tview.NewList()
			list.SetBorder(true).SetTitle("Packages")
			for _, p := range packages {
				list.AddItem(p, "", 0, nil)
			}
			list.SetSelectedFunc(func(index int, mainText string, secondaryText string, shortcut rune) {
				// Create a new text view for the gutter
				gutter := tview.NewTextView()
				gutter.SetText("Navigation info")
				gutter.SetBorder(true).SetTitle("Provided files")
				app.SetRoot(gutter, true)
			})
			app.SetRoot(list, true)
		}
	})

	if err := app.SetRoot(flex, true).SetFocus(flex).Run(); err != nil {
		log.Fatalf("could not start program: %s", err.Error())
	}

	// Run the application
	if err := app.Run(); err != nil {
		log.Fatalf("could not start program: %s", err)
	}
}
