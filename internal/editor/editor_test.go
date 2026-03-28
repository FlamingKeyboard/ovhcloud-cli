// SPDX-FileCopyrightText: 2025 OVH SAS <opensource@ovh.net>
//
// SPDX-License-Identifier: Apache-2.0

package editor

import (
	"testing"

	"github.com/maxatome/go-testdeep/td"
)

func TestDefaultEditorForOS(t *testing.T) {
	td.Cmp(t, defaultEditorForOS("windows"), "notepad")
	td.Cmp(t, defaultEditorForOS("linux"), "vi")
}

func TestCommandForEditor_Windows(t *testing.T) {
	t.Setenv("COMSPEC", `C:\Windows\System32\cmd.exe`)

	cmd := commandForEditor("windows", "code --wait", `C:\Users\alice\AppData\Local\Temp\ovh cli edit.json`)

	td.Cmp(t, cmd.Path, `C:\Windows\System32\cmd.exe`)
	td.Cmp(t, cmd.Args, []string{
		`C:\Windows\System32\cmd.exe`,
		"/c",
		`code --wait "C:\Users\alice\AppData\Local\Temp\ovh cli edit.json"`,
	})
}

func TestCommandForEditor_WindowsQuotesExecutablePath(t *testing.T) {
	t.Setenv("COMSPEC", `C:\Windows\System32\cmd.exe`)

	cmd := commandForEditor("windows", `C:\Program Files\Microsoft VS Code\Code.exe --wait`, `C:\Users\alice\AppData\Local\Temp\ovh cli edit.json`)

	td.Cmp(t, cmd.Args, []string{
		`C:\Windows\System32\cmd.exe`,
		"/c",
		`"C:\Program Files\Microsoft VS Code\Code.exe" --wait "C:\Users\alice\AppData\Local\Temp\ovh cli edit.json"`,
	})
}

func TestCommandForEditor_Unix(t *testing.T) {
	cmd := commandForEditor("linux", "code --wait", "/tmp/ovh cli edit.json")

	td.Cmp(t, cmd.Path, "sh")
	td.Cmp(t, cmd.Args, []string{
		"sh",
		"-c",
		`code --wait '/tmp/ovh cli edit.json'`,
	})
}

func TestQuoteEditorFilename_UnixEscapesSingleQuote(t *testing.T) {
	quoted := quoteEditorFilename("linux", `/tmp/ovh'cli.json`)

	td.Cmp(t, quoted, `'/tmp/ovh'"'"'cli.json'`)
}

func TestQuoteWindowsEditor(t *testing.T) {
	td.Cmp(t, quoteWindowsEditor(`C:\Program Files\Notepad++\notepad++.exe -multiInst`), `"C:\Program Files\Notepad++\notepad++.exe" -multiInst`)
	td.Cmp(t, quoteWindowsEditor(`"C:\Program Files\Notepad++\notepad++.exe" -multiInst`), `"C:\Program Files\Notepad++\notepad++.exe" -multiInst`)
	td.Cmp(t, quoteWindowsEditor(`code --wait`), `code --wait`)
}
