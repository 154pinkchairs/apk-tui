package main

import (
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

type (
	PackageInfo struct {
		Name     string
		Provides []string
	}
	model struct {
		pkgs        []string
		cursor      int
		choice      string
		searchQuery string
		visiblePkgs []string
		gutter      string
		mode        string // list, search, info
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

func (m model) Init() tea.Cmd {
	return tea.Batch(
		tea.EnterAltScreen,
	)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch m.mode {
		case "list":
			switch msg.String() {
			case "ctrl+c", "q":
				return m, tea.Quit
			case "up", "j":
				if m.cursor > 0 {
					m.cursor--
				}
			case "down", "k":
				if m.cursor < len(m.visiblePkgs)-1 {
					m.cursor++
				}
			case "enter", " ":
				m.choice = m.visiblePkgs[m.cursor]
			case "/":
				m.mode = "search"
				m.searchQuery = ""
			}
		case "search":
			switch msg.String() {
			case "enter", " ":
				m.mode = "list"
				m.searchAndUpdateVisiblePkgs()
			default:
				m.searchQuery += msg.String()
			}
		}
	}
	return m, nil
}

func (m *model) View() string {
	switch m.mode {
	case "list":
		return m.listView()
	case "search":
		return m.searchView()
	default:
		return "Unknown mode"
	}
}

func (m *model) listView() string {
	m.gutter = fmt.Sprintf("Cursor: %d/%d", m.cursor+1, len(m.visiblePkgs))

	s := m.gutter + "\n\nChoose a package:\n\n"
	for i, p := range m.visiblePkgs {
		if i == m.cursor {
			s += "> "
		} else {
			s += "  "
		}
		s += p + "\n"
	}
	s += "\n\nSelected: " + m.choice

	return s
}

func (m *model) searchView() string {
	return fmt.Sprintf("Search: %s", m.searchQuery)
}

func (m *model) searchAndUpdateVisiblePkgs() {
	m.visiblePkgs = []string{}
	for _, pkg := range m.pkgs {
		if strings.Contains(pkg, m.searchQuery) {
			m.visiblePkgs = append(m.visiblePkgs, pkg)
		}
	}
}

func worker(jobs <-chan string, results chan<- PackageInfo, done <-chan bool) {
	for {
		select {
		case j, more := <-jobs:
			if more {
				results <- PackageInfo{Name: j, Provides: getPackageProvides(j)}
			} else {
				return
			}
		case <-done:
			return
		}
	}
}

func main() {
	packages, err := getPackages()
	if err != nil {
		log.Fatal(err)
	}

	jobs := make(chan string, len(packages))
	results := make(chan PackageInfo, len(packages))

	done := make(chan bool)
	defer close(done)

	var wg sync.WaitGroup
	wg.Add(runtime.NumCPU())
	for w := 1; w <= runtime.NumCPU(); w++ {
		go func() {
			worker(jobs, results, done)
			wg.Done()
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	go func() {
		for _, p := range packages {
			jobs <- p
		}
		close(jobs)
	}()

	packagesMap := &sync.Map{}
	go func() {
		for res := range results {
			packagesMap.Store(res.Name, res.Provides)
		}
	}()

	initialModel := model{
		pkgs:        packages,
		cursor:      0,
		choice:      "",
		visiblePkgs: packages,
		mode:        "list",
	}

	p := tea.NewProgram(&initialModel)
	if _, err := p.Run(); err != nil {
		log.Fatalf("could not start program: %s", err)
	}
}
