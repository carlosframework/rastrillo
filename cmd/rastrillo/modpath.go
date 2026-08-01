package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// modulePath reads the module directive from dir/go.mod. Hand-rolled
// rather than importing golang.org/x/mod: the module line is one fixed
// shape, this is a page of code, not an SDK's worth — the family's own
// convention for when to hand-roll (carlosframework/skills, blueprint.md).
func modulePath(dir string) (string, error) {
	f, err := os.Open(dir + "/go.mod")
	if err != nil {
		return "", fmt.Errorf("read go.mod: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("go.mod: no module directive found")
}
