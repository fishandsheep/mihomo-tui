package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func TestRunVersionCommands(t *testing.T) {
	oldVersion := version
	version = "1.2.3"
	t.Cleanup(func() { version = oldVersion })

	for _, args := range [][]string{
		{"--version"},
		{"version"},
	} {
		stdout, stderr, err := captureRunOutput(t, args)
		if err != nil {
			t.Fatalf("run(%v) failed: %v", args, err)
		}
		if stdout != "mihomo-tui 1.2.3\n" {
			t.Fatalf("run(%v) stdout = %q", args, stdout)
		}
		if stderr != "" {
			t.Fatalf("run(%v) stderr = %q", args, stderr)
		}
	}
}

func TestHelpUsageUsesMihomoTUIName(t *testing.T) {
	stdout, stderr, err := captureRunOutput(t, []string{"help"})
	if err != nil {
		t.Fatalf("help failed: %v", err)
	}
	if !strings.Contains(stdout, "mihomo-tui open --profile <name>") {
		t.Fatalf("help output missing binary name: %q", stdout)
	}
	if !strings.Contains(stdout, "mihomo-tui version") {
		t.Fatalf("help output missing version command: %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("help stderr = %q", stderr)
	}
}

func captureRunOutput(t *testing.T, args []string) (string, string, error) {
	t.Helper()

	origStdout := os.Stdout
	origStderr := os.Stderr

	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}

	os.Stdout = stdoutWriter
	os.Stderr = stderrWriter

	runErr := run(args)

	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	os.Stdout = origStdout
	os.Stderr = origStderr

	stdout := readPipe(t, stdoutReader)
	stderr := readPipe(t, stderrReader)
	return stdout, stderr, runErr
}

func readPipe(t *testing.T, reader *os.File) string {
	t.Helper()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	return buf.String()
}
