// SPDX-FileCopyrightText: 2025 OVH SAS <opensource@ovh.net>
//
// SPDX-License-Identifier: Apache-2.0

package editor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func defaultEditorForOS(goos string) string {
	if goos == "windows" {
		return "notepad"
	}

	return "vi"
}

func quoteEditorFilename(goos, filename string) string {
	switch goos {
	case "windows":
		return `"` + strings.ReplaceAll(filename, `"`, `""`) + `"`
	default:
		return `'` + strings.ReplaceAll(filename, `'`, `'"'"'`) + `'`
	}
}

func quoteWindowsEditor(editor string) string {
	trimmedEditor := strings.TrimSpace(editor)
	if strings.HasPrefix(trimmedEditor, `"`) && strings.Contains(trimmedEditor[1:], `"`) {
		return editor
	}

	loweredEditor := strings.ToLower(editor)
	for _, extension := range []string{".exe", ".cmd", ".bat", ".com"} {
		index := strings.Index(loweredEditor, extension)
		if index < 0 {
			continue
		}

		index += len(extension)
		executable := editor[:index]
		if filepath.Base(executable) == executable || strings.ContainsAny(executable, " \t") {
			return `"` + strings.ReplaceAll(executable, `"`, `""`) + `"` + editor[index:]
		}

		return editor
	}

	return editor
}

func commandForEditor(goos, editor, filename string) *exec.Cmd {
	quotedFilename := quoteEditorFilename(goos, filename)

	switch goos {
	case "windows":
		comspec := os.Getenv("COMSPEC")
		if comspec == "" {
			comspec = "cmd"
		}
		editor = quoteWindowsEditor(editor)
		return exec.Command(comspec, "/c", editor+" "+quotedFilename)
	default:
		return exec.Command("sh", "-c", editor+" "+quotedFilename)
	}
}

func EditValueWithEditor(value []byte) ([]byte, error) {
	editor := defaultEditorForOS(runtime.GOOS)
	if s := os.Getenv("EDITOR"); s != "" {
		editor = s
	}

	// Create temp file
	f, err := os.CreateTemp("", "ovh-cli-edit")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(f.Name())

	if _, err := f.Write(value); err != nil {
		return nil, fmt.Errorf("failed to write input file: %w", err)
	}

	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("failed to close temp file: %w", err)
	}

	// Open editor
	cmd := commandForEditor(runtime.GOOS, editor, f.Name())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to edit file: %w", err)
	}

	// Read updated file
	b, err := os.ReadFile(f.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to read updated file: %w", err)
	}

	return b, nil
}
