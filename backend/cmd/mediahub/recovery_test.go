package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

func passwordInput(t *testing.T, text string) *os.File {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "input")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(text); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Error(err)
		}
	})
	return file
}

func TestExplicitStdinPasswordInput(t *testing.T) {
	for _, test := range []struct {
		input string
		valid bool
		want  string
	}{

		{strings.Repeat("A", 32) + "\n", true, strings.Repeat("A", 32)},
		{strings.Repeat("A", 32) + "\r\n", true, strings.Repeat("A", 32)},
		{"!@#$%^&*()_+-=[]{};:,.<>/?\\|`~\"'\n", true, "!@#$%^&*()_+-=[]{};:,.<>/?\\|`~\"'"},
		{"LongPass123!\r\n", true, "LongPass123!"},
		{strings.Repeat("A", 33) + "\n", false, ""},
		{"  spaces preserved  \n", false, ""},
		{"LongPass123!\r", false, ""},
		{"LongPass123!\x7f\n", false, ""},
		{"LongPass123!\t\n", false, ""},
		{"password\nsecond password\n", false, ""},
		{"short\n", false, ""},
		{"密码Password123\n", false, ""},
	} {
		t.Run(test.input, func(t *testing.T) {
			password, err := readNewPassword(passwordInput(t, test.input), io.Discard, true)
			if (err == nil) != test.valid || password != test.want {
				t.Fatalf("input validation: valid=%v err=%v", password != "", err)
			}
		})
	}
	if _, err := readNewPassword(passwordInput(t, "LongPass123!"), io.Discard, false); err == nil || !strings.Contains(err.Error(), "--password-stdin") {
		t.Fatal("implicit piped input accepted")
	}
}
