package skills

import (
	"os"
	"path/filepath"
	"strings"
)

type Skill struct {
	Name    string
	Summary string
	Body    string
}

type Set []Skill

func LoadDir(dir string) (Set, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var set Set
	for _, entry := range entries {
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".md" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		body := string(data)
		set = append(set, Skill{
			Name:    strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())),
			Summary: firstNonEmptyLine(body),
			Body:    body,
		})
	}
	return set, nil
}

func firstNonEmptyLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if line != "" {
			return line
		}
	}
	return "no summary"
}
