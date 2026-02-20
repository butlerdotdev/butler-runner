// Copyright 2026 The Butler Authors.
// SPDX-License-Identifier: Apache-2.0

package terraform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/butlerdotdev/butler-runner/internal/config"
)

// RunResult contains the result of a terraform operation.
type RunResult struct {
	ExitCode           int
	ResourcesToAdd     int
	ResourcesToChange  int
	ResourcesToDestroy int
	PlanJSON           string
	PlanText           string
	Outputs            map[string]interface{}
}

// Executor runs terraform commands in a working directory.
type Executor struct {
	tfPath     string
	workingDir string
	logger     *slog.Logger
	stdout     io.Writer // optional: tee stdout to this writer
	stderr     io.Writer // optional: tee stderr to this writer
}

// NewExecutor creates a new terraform executor.
func NewExecutor(tfPath, workingDir string, logger *slog.Logger) *Executor {
	return &Executor{
		tfPath:     tfPath,
		workingDir: workingDir,
		logger:     logger,
	}
}

// SetLogWriters sets optional writers that receive copies of terraform stdout/stderr.
func (e *Executor) SetLogWriters(stdout, stderr io.Writer) {
	e.stdout = stdout
	e.stderr = stderr
}

// Init runs terraform init.
func (e *Executor) Init(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, e.tfPath, "init", "-input=false", "-no-color")
	cmd.Dir = e.workingDir
	cmd.Env = append(os.Environ(), "TF_IN_AUTOMATION=1")

	var stderr bytes.Buffer
	if e.stderr != nil {
		cmd.Stderr = io.MultiWriter(&stderr, e.stderr)
	} else {
		cmd.Stderr = &stderr
	}
	if e.stdout != nil {
		cmd.Stdout = io.MultiWriter(os.Stdout, e.stdout)
	} else {
		cmd.Stdout = os.Stdout
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("terraform init failed: %s: %w", stderr.String(), err)
	}
	return nil
}

// Run executes the given terraform operation (plan, apply, destroy).
func (e *Executor) Run(ctx context.Context, operation string) (*RunResult, error) {
	switch operation {
	case "plan":
		return e.plan(ctx)
	case "apply":
		return e.apply(ctx)
	case "destroy":
		return e.destroy(ctx)
	default:
		return nil, fmt.Errorf("unsupported operation: %s", operation)
	}
}

func (e *Executor) plan(ctx context.Context) (*RunResult, error) {
	planFile := filepath.Join(e.workingDir, "tfplan")

	cmd := exec.CommandContext(ctx, e.tfPath, "plan", "-input=false", "-no-color", "-out="+planFile)
	cmd.Dir = e.workingDir
	cmd.Env = append(os.Environ(), "TF_IN_AUTOMATION=1")

	var stdout, stderr bytes.Buffer
	if e.stdout != nil {
		cmd.Stdout = io.MultiWriter(&stdout, e.stdout)
	} else {
		cmd.Stdout = &stdout
	}
	if e.stderr != nil {
		cmd.Stderr = io.MultiWriter(&stderr, e.stderr)
	} else {
		cmd.Stderr = &stderr
	}

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			// Exit code 2 = changes present (not an error for plan)
			if exitCode == 2 {
				err = nil
			}
		}
	}

	result := &RunResult{
		ExitCode: exitCode,
		PlanText: stdout.String(),
	}

	// Get plan JSON
	if _, statErr := os.Stat(planFile); statErr == nil {
		showCmd := exec.CommandContext(ctx, e.tfPath, "show", "-json", planFile)
		showCmd.Dir = e.workingDir
		var showOut bytes.Buffer
		showCmd.Stdout = &showOut
		if showErr := showCmd.Run(); showErr == nil {
			result.PlanJSON = showOut.String()
			e.parseResourceCounts(result)
		}
	}

	if err != nil {
		return result, fmt.Errorf("terraform plan: %s: %w", stderr.String(), err)
	}
	return result, nil
}

func (e *Executor) apply(ctx context.Context) (*RunResult, error) {
	cmd := exec.CommandContext(ctx, e.tfPath, "apply", "-input=false", "-no-color", "-auto-approve")
	cmd.Dir = e.workingDir
	cmd.Env = append(os.Environ(), "TF_IN_AUTOMATION=1")

	var stdout, stderr bytes.Buffer
	if e.stdout != nil {
		cmd.Stdout = io.MultiWriter(&stdout, e.stdout)
	} else {
		cmd.Stdout = &stdout
	}
	if e.stderr != nil {
		cmd.Stderr = io.MultiWriter(&stderr, e.stderr)
	} else {
		cmd.Stderr = &stderr
	}

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	result := &RunResult{
		ExitCode: exitCode,
	}
	parseSummaryCounts(stdout.String(), result)

	// Get outputs
	outputCmd := exec.CommandContext(ctx, e.tfPath, "output", "-json")
	outputCmd.Dir = e.workingDir
	var outputBuf bytes.Buffer
	outputCmd.Stdout = &outputBuf
	if outputErr := outputCmd.Run(); outputErr == nil {
		var outputs map[string]interface{}
		if jsonErr := json.Unmarshal(outputBuf.Bytes(), &outputs); jsonErr == nil {
			result.Outputs = outputs
		}
	}

	if err != nil {
		return result, fmt.Errorf("terraform apply: %s: %w", stderr.String(), err)
	}
	return result, nil
}

func (e *Executor) destroy(ctx context.Context) (*RunResult, error) {
	cmd := exec.CommandContext(ctx, e.tfPath, "destroy", "-input=false", "-no-color", "-auto-approve")
	cmd.Dir = e.workingDir
	cmd.Env = append(os.Environ(), "TF_IN_AUTOMATION=1")

	var stdout, stderr bytes.Buffer
	if e.stdout != nil {
		cmd.Stdout = io.MultiWriter(&stdout, e.stdout)
	} else {
		cmd.Stdout = &stdout
	}
	if e.stderr != nil {
		cmd.Stderr = io.MultiWriter(&stderr, e.stderr)
	} else {
		cmd.Stderr = &stderr
	}

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
	}

	result := &RunResult{
		ExitCode: exitCode,
	}
	parseSummaryCounts(stdout.String(), result)

	if err != nil {
		return result, fmt.Errorf("terraform destroy: %s: %w", stderr.String(), err)
	}
	return result, nil
}

func (e *Executor) parseResourceCounts(result *RunResult) {
	if result.PlanJSON == "" {
		return
	}
	var plan struct {
		ResourceChanges []struct {
			Change struct {
				Actions []string `json:"actions"`
			} `json:"change"`
		} `json:"resource_changes"`
	}
	if err := json.Unmarshal([]byte(result.PlanJSON), &plan); err != nil {
		return
	}
	for _, rc := range plan.ResourceChanges {
		actions := strings.Join(rc.Change.Actions, ",")
		switch {
		case actions == "create":
			result.ResourcesToAdd++
		case actions == "update":
			result.ResourcesToChange++
		case actions == "delete":
			result.ResourcesToDestroy++
		case strings.Contains(actions, "create") && strings.Contains(actions, "delete"):
			result.ResourcesToDestroy++
			result.ResourcesToAdd++
		}
	}
}

// WriteTfvars writes variables and upstream outputs to a terraform.tfvars.json file.
func WriteTfvars(workDir string, variables map[string]config.Variable, upstreamOutputs map[string]interface{}) (string, error) {
	tfvars := make(map[string]interface{})

	for key, v := range variables {
		tfvars[key] = v.Value
	}
	for key, v := range upstreamOutputs {
		tfvars[key] = v
	}

	data, err := json.MarshalIndent(tfvars, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshaling tfvars: %w", err)
	}

	path := filepath.Join(workDir, "terraform.tfvars.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("writing tfvars: %w", err)
	}

	return path, nil
}

// SecureDelete overwrites a file with zeros before deleting it.
func SecureDelete(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	zeros := make([]byte, info.Size())
	_ = os.WriteFile(path, zeros, 0o600)
	_ = os.Remove(path)
}

// parseSummaryCounts extracts resource counts from terraform apply/destroy
// summary lines such as:
//
//	"Apply complete! Resources: 1 added, 0 changed, 0 destroyed."
//	"Destroy complete! Resources: 3 destroyed."
var summaryRe = regexp.MustCompile(`(\d+) (added|changed|destroyed)`)

func parseSummaryCounts(output string, result *RunResult) {
	for _, match := range summaryRe.FindAllStringSubmatch(output, -1) {
		n, _ := strconv.Atoi(match[1])
		switch match[2] {
		case "added":
			result.ResourcesToAdd = n
		case "changed":
			result.ResourcesToChange = n
		case "destroyed":
			result.ResourcesToDestroy = n
		}
	}
}

// Variable is an alias for config.Variable.
type Variable = config.Variable
