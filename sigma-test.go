package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/bradleyjkemp/sigma-go"
	"github.com/bradleyjkemp/sigma-go/evaluator"
	"gopkg.in/yaml.v3"
)

var (
	fRecursive = flag.Bool("recursive", true, "whether to test directories recursively")
)

func main() {
	flag.Parse()
	paths := flag.Args()
	if len(paths) == 0 {
		paths = []string{"."}
	}

	allPassed := true
	for _, path := range paths {
		pass, err := run(path, *fRecursive)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		allPassed = allPassed && pass
	}

	if !allPassed {
		os.Exit(1)
	}
}

func run(root string, recursive bool) (bool, error) {
	results := tabwriter.NewWriter(os.Stdout, 0, 0, 4, ' ', 0)
	passed := true

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			if path != root && !recursive {
				return filepath.SkipDir
			}
			return nil
		}

		if filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" {
			return nil
		}

		contents, err := ioutil.ReadFile(path)
		if err != nil {
			return fmt.Errorf("error reading %s: %w", path, err)
		}

		rule, match, dontMatch, err := parseRule(contents)
		if err != nil {
			return fmt.Errorf("error parsing %s: %w", path, err)
		}

		err, failures := testFile(rule, match, dontMatch)
		if err != nil {
			if errors.Is(err, errFailedTests) {
				passed = false
			}
			fmt.Fprintf(results, "%s\t%v\t\n", path, err)
			for _, failure := range failures {
				fmt.Fprintf(results, "\t%v\n", failure)
			}
		} else {
			fmt.Fprintf(results, "%s\tPASS\t\n", path)
		}
		return nil
	})

	results.Flush()
	return passed, err
}

var (
	errNoTests     = fmt.Errorf("SKIP")
	errFailedTests = fmt.Errorf("FAIL")
)

func testFile(r sigma.Rule, match, dontMatch []map[string]interface{}) (error, []string) {
	if len(match) == 0 && len(dontMatch) == 0 {
		return errNoTests, nil
	}
	rule := evaluator.ForRule(r)
	pass := true
	var failures []string

	for _, matchCase := range match {
		// TODO: what happens with aggregations...?
		if result, _ := rule.Matches(context.Background(), matchCase); result.Match == false {
			pass = false
			failures = append(failures, fmt.Sprintf("%v should have matched", matchCase))
		}
	}

	for _, dontMatchCase := range dontMatch {
		// TODO: what happens with aggregations...?
		if result, _ := rule.Matches(context.Background(), dontMatchCase); result.Match {
			pass = false
			failures = append(failures, fmt.Sprintf("%v shouldn't have matched", dontMatchCase))
		}
	}

	if pass {
		return nil, nil
	}
	return errFailedTests, failures
}

type TestCases struct {
	Cases struct {
		Match     []map[string]interface{} `yaml:"match"`
		DontMatch []map[string]interface{} `yaml:"dont-match"`
	} `yaml:"testcases"`
}

func parseRule(contents []byte) (rule sigma.Rule, match []map[string]interface{}, dontMatch []map[string]interface{}, err error) {
	rule, err = sigma.ParseRule(contents)
	if err != nil {
		return sigma.Rule{}, nil, nil, fmt.Errorf("failed to parse Rule: %w", err)
	}

	tc := TestCases{}
	err = yaml.Unmarshal(contents, &tc)
	if err != nil {
		return sigma.Rule{}, nil, nil, fmt.Errorf("failed to parse test cases: %w", err)
	}
	return rule, tc.Cases.Match, tc.Cases.DontMatch, nil
}
