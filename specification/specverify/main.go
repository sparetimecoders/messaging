// MIT License
//
// Copyright (c) 2026 sparetimecoders
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/sparetimecoders/gomessaging/spec"
)

const usage = `Usage: specverify <command> [flags] [args...]

Commands:
  validate        Validate a single topology file (or stdin with -)
  cross-validate  Cross-validate multiple topology files
  check-fixtures  Verify all JSON files in a testdata directory
  visualize       Generate a Mermaid diagram from topology files
  discover        Discover topology from a running RabbitMQ broker

Flags:
  --format text|json  Output format (default: text)
  --verbose           Show detailed output on success
  --no-validate       Skip validation before visualizing (visualize only)

Discover flags:
  --url               RabbitMQ Management API URL (default: http://localhost:15672)
  --user              Username (default: guest)
  --password          Password (default: guest)
  --vhost             Vhost (default: /)
  --output            Output mode: mermaid, json, or topology-dir (default: mermaid)
`

type config struct {
	format     string
	verbose    bool
	noValidate bool
	// discover-specific
	brokerURL      string
	brokerUser     string
	brokerPassword string
	brokerVhost    string
	output         string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 1
	}

	cmd := args[0]
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)

	var cfg config
	fs.StringVar(&cfg.format, "format", "text", "output format: text or json")
	fs.BoolVar(&cfg.verbose, "verbose", false, "show detailed output on success")
	fs.BoolVar(&cfg.noValidate, "no-validate", false, "skip validation before visualizing")
	fs.StringVar(&cfg.brokerURL, "url", "http://localhost:15672", "RabbitMQ Management API URL")
	fs.StringVar(&cfg.brokerUser, "user", "guest", "RabbitMQ Management API username")
	fs.StringVar(&cfg.brokerPassword, "password", "guest", "RabbitMQ Management API password")
	fs.StringVar(&cfg.brokerVhost, "vhost", "/", "RabbitMQ vhost")
	fs.StringVar(&cfg.output, "output", "mermaid", "discover output: mermaid, json, or topology-dir")

	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}

	if cfg.format != "text" && cfg.format != "json" {
		fmt.Fprintf(stderr, "error: --format must be text or json, got %q\n", cfg.format)
		return 1
	}

	switch cmd {
	case "validate":
		return cmdValidate(cfg, fs.Args(), stdin, stdout, stderr)
	case "cross-validate":
		return cmdCrossValidate(cfg, fs.Args(), stdin, stdout, stderr)
	case "check-fixtures":
		return cmdCheckFixtures(cfg, fs.Args(), stdout, stderr)
	case "visualize":
		return cmdVisualize(cfg, fs.Args(), stdin, stdout, stderr)
	case "discover":
		return cmdDiscover(cfg, fs.Args(), stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n%s", cmd, usage)
		return 1
	}
}

func cmdValidate(cfg config, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var r io.Reader
	var name string

	switch len(args) {
	case 0:
		name = "<stdin>"
		r = stdin
	case 1:
		if args[0] == "-" {
			name = "<stdin>"
			r = stdin
		} else {
			name = args[0]
			f, err := os.Open(args[0])
			if err != nil {
				writeResult(cfg, stdout, result{OK: false, Errors: []string{err.Error()}})
				return 1
			}
			defer f.Close()
			r = f
		}
	default:
		fmt.Fprintln(stderr, "validate expects zero or one argument (file path or - for stdin)")
		return 1
	}

	data, err := io.ReadAll(r)
	if err != nil {
		writeResult(cfg, stdout, result{OK: false, Errors: []string{fmt.Sprintf("reading %s: %s", name, err)}})
		return 1
	}

	var topo spec.Topology
	if err := json.Unmarshal(data, &topo); err != nil {
		writeResult(cfg, stdout, result{OK: false, Errors: []string{fmt.Sprintf("parsing %s: %s", name, err)}})
		return 1
	}

	if err := spec.Validate(topo); err != nil {
		errs := splitErrors(err)
		writeResult(cfg, stdout, result{OK: false, Errors: errs})
		return 1
	}

	res := result{OK: true}
	if cfg.verbose {
		res.Details = fmt.Sprintf("%s: %d endpoints OK", topo.ServiceName, len(topo.Endpoints))
	}
	writeResult(cfg, stdout, res)
	return 0
}

func cmdCrossValidate(cfg config, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "cross-validate expects one or more topology file paths")
		return 1
	}

	var topologies []spec.Topology
	for _, path := range args {
		var r io.Reader
		name := path
		if path == "-" {
			name = "<stdin>"
			r = stdin
		} else {
			f, err := os.Open(path)
			if err != nil {
				writeResult(cfg, stdout, result{OK: false, Errors: []string{err.Error()}})
				return 1
			}
			defer f.Close()
			r = f
		}

		data, err := io.ReadAll(r)
		if err != nil {
			writeResult(cfg, stdout, result{OK: false, Errors: []string{fmt.Sprintf("reading %s: %s", name, err)}})
			return 1
		}

		var topo spec.Topology
		if err := json.Unmarshal(data, &topo); err != nil {
			writeResult(cfg, stdout, result{OK: false, Errors: []string{fmt.Sprintf("parsing %s: %s", name, err)}})
			return 1
		}
		topologies = append(topologies, topo)
	}

	if err := spec.ValidateTopologies(topologies); err != nil {
		errs := splitErrors(err)
		writeResult(cfg, stdout, result{OK: false, Errors: errs})
		return 1
	}

	res := result{OK: true}
	if cfg.verbose {
		var names []string
		totalEndpoints := 0
		for _, t := range topologies {
			names = append(names, t.ServiceName)
			totalEndpoints += len(t.Endpoints)
		}
		res.Details = fmt.Sprintf("%d services (%s), %d endpoints OK", len(topologies), strings.Join(names, ", "), totalEndpoints)
	}
	writeResult(cfg, stdout, res)
	return 0
}

func cmdCheckFixtures(cfg config, args []string, stdout, stderr io.Writer) int {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		writeResult(cfg, stdout, result{OK: false, Errors: []string{fmt.Sprintf("reading directory %s: %s", dir, err)}})
		return 1
	}

	var allErrors []string
	checked := 0

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		checked++
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			allErrors = append(allErrors, fmt.Sprintf("%s: %s", entry.Name(), err))
			continue
		}

		// Try parsing as Topology first
		var topo spec.Topology
		if err := json.Unmarshal(data, &topo); err != nil {
			// Not a topology file — that's OK, just check it's valid JSON
			var raw json.RawMessage
			if err := json.Unmarshal(data, &raw); err != nil {
				allErrors = append(allErrors, fmt.Sprintf("%s: invalid JSON: %s", entry.Name(), err))
			}
			continue
		}

		if topo.ServiceName == "" {
			// Not a topology file (e.g. naming.json, constants.json)
			continue
		}

		if err := spec.Validate(topo); err != nil {
			allErrors = append(allErrors, fmt.Sprintf("%s: %s", entry.Name(), err))
		}
	}

	if len(allErrors) > 0 {
		writeResult(cfg, stdout, result{OK: false, Errors: allErrors})
		return 1
	}

	res := result{OK: true}
	if cfg.verbose {
		res.Details = fmt.Sprintf("%d JSON files checked in %s", checked, dir)
	}
	writeResult(cfg, stdout, res)
	return 0
}

func cmdDiscover(cfg config, args []string, stdout, stderr io.Writer) int {
	topologies, err := spec.DiscoverTopologies(spec.BrokerConfig{
		URL:      cfg.brokerURL,
		Username: cfg.brokerUser,
		Password: cfg.brokerPassword,
		Vhost:    cfg.brokerVhost,
		Client:   &http.Client{},
	})
	if err != nil {
		writeResult(cfg, stdout, result{OK: false, Errors: []string{err.Error()}})
		return 1
	}

	switch cfg.output {
	case "mermaid":
		diagram := spec.Mermaid(topologies)
		if cfg.format == "json" {
			type vizResult struct {
				OK      bool   `json:"ok"`
				Diagram string `json:"diagram"`
			}
			enc := json.NewEncoder(stdout)
			enc.SetIndent("", "  ")
			enc.Encode(vizResult{OK: true, Diagram: diagram}) //nolint:errcheck
		} else {
			fmt.Fprint(stdout, diagram)
		}
	case "json":
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		enc.Encode(topologies) //nolint:errcheck
	case "topology-dir":
		if len(args) == 0 {
			fmt.Fprintln(stderr, "discover --output=topology-dir requires a directory argument")
			return 1
		}
		dir := args[0]
		if err := os.MkdirAll(dir, 0755); err != nil {
			writeResult(cfg, stdout, result{OK: false, Errors: []string{err.Error()}})
			return 1
		}
		for _, topo := range topologies {
			data, err := json.MarshalIndent(topo, "", "  ")
			if err != nil {
				writeResult(cfg, stdout, result{OK: false, Errors: []string{err.Error()}})
				return 1
			}
			path := filepath.Join(dir, topo.ServiceName+".json")
			if err := os.WriteFile(path, data, 0644); err != nil {
				writeResult(cfg, stdout, result{OK: false, Errors: []string{err.Error()}})
				return 1
			}
			fmt.Fprintf(stderr, "wrote %s\n", path)
		}
		if cfg.verbose {
			fmt.Fprintf(stdout, "OK: %d services discovered, written to %s\n", len(topologies), dir)
		}
	default:
		fmt.Fprintf(stderr, "unknown output mode: %q (use mermaid, json, or topology-dir)\n", cfg.output)
		return 1
	}
	return 0
}

func cmdVisualize(cfg config, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "visualize expects one or more topology file paths")
		return 1
	}

	var topologies []spec.Topology
	for _, path := range args {
		var r io.Reader
		name := path
		if path == "-" {
			name = "<stdin>"
			r = stdin
		} else {
			f, err := os.Open(path)
			if err != nil {
				writeResult(cfg, stdout, result{OK: false, Errors: []string{err.Error()}})
				return 1
			}
			defer f.Close()
			r = f
		}

		data, err := io.ReadAll(r)
		if err != nil {
			writeResult(cfg, stdout, result{OK: false, Errors: []string{fmt.Sprintf("reading %s: %s", name, err)}})
			return 1
		}

		var topo spec.Topology
		if err := json.Unmarshal(data, &topo); err != nil {
			writeResult(cfg, stdout, result{OK: false, Errors: []string{fmt.Sprintf("parsing %s: %s", name, err)}})
			return 1
		}
		topologies = append(topologies, topo)
	}

	if !cfg.noValidate {
		if err := spec.ValidateTopologies(topologies); err != nil {
			errs := splitErrors(err)
			writeResult(cfg, stdout, result{OK: false, Errors: errs})
			return 1
		}
	}

	diagram := spec.Mermaid(topologies)

	if cfg.format == "json" {
		type vizResult struct {
			OK      bool   `json:"ok"`
			Diagram string `json:"diagram"`
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		enc.Encode(vizResult{OK: true, Diagram: diagram}) //nolint:errcheck
		return 0
	}

	fmt.Fprint(stdout, diagram)
	return 0
}

type result struct {
	OK      bool     `json:"ok"`
	Errors  []string `json:"errors,omitempty"`
	Details string   `json:"details,omitempty"`
}

func writeResult(cfg config, w io.Writer, res result) {
	if cfg.format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(res) //nolint:errcheck
		return
	}

	if res.OK {
		if res.Details != "" {
			fmt.Fprintln(w, "OK:", res.Details)
		} else {
			fmt.Fprintln(w, "OK")
		}
		return
	}

	for _, e := range res.Errors {
		fmt.Fprintln(w, "ERROR:", e)
	}
}

func splitErrors(err error) []string {
	parts := strings.Split(err.Error(), "\n")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
