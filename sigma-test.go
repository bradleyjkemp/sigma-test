package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/bradleyjkemp/sigma-go"
	"github.com/bradleyjkemp/sigma-go/evaluator"
	"gopkg.in/yaml.v3"
)

var (
	fRecursive   = flag.Bool("recursive", true, "whether to test directories recursively")
	fConfigFiles = flag.String("config-files", "", "a pattern for config files to use when evaluating rules")
)

func main() {
	flag.Parse()
	paths := flag.Args()
	if len(paths) == 0 {
		paths = []string{"."}
	}

	configs, err := loadConfigs()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	allPassed := true
	for _, path := range paths {
		pass, err := run(path, configs, *fRecursive)
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

func run(root string, configs []sigma.Config, recursive bool) (bool, error) {
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

		contents, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("error reading %s: %w", path, err)
		}

		if sigma.InferFileType(contents) != sigma.RuleFile {
			return nil
		}
		rule, err := sigma.ParseRule(contents)
		if err != nil {
			return fmt.Errorf("error parsing %s: %w", path, err)
		}

		err, failures := testFile(path, rule, configs)
		if err != nil {
			if errors.Is(err, errFailedTests) || errors.Is(err, errNoLogSources) {
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

func loadConfigs() ([]sigma.Config, error) {
	if *fConfigFiles == "" {
		return nil, nil
	}
	var configs []sigma.Config
	configFilepaths, err := filepath.Glob(*fConfigFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to identify config files: %w", err)
	}

	for _, configFilepath := range configFilepaths {
		configBytes, err := os.ReadFile(configFilepath)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file %s: %w", configFilepath, err)
		}

		config, err := sigma.ParseConfig(configBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse config file %s: %w", configFilepath, err)
		}

		for _, backend := range config.Backends {
			if backend == "github.com/bradleyjkemp/sigma-go" {
				configs = append(configs, config)
				break
			}
		}
	}

	return configs, nil
}

var (
	errNoTests      = fmt.Errorf("SKIP")
	errFailedTests  = fmt.Errorf("FAIL")
	errNoLogSources = fmt.Errorf("ERROR: No relevant logsource configurations")
)

func testFile(path string, r sigma.Rule, configs []sigma.Config) (error, []string) {
	ext := filepath.Ext(path)
	testFilename := strings.TrimSuffix(path, ext) + "_test" + ext

	testCases, err := getTestCases(testFilename)
	if err != nil {
		return err, nil
	}
	if len(testCases) == 0 {
		return errNoTests, nil
	}

	// only use logsources that are relevant for this rule. This avoids having conflicts with other logsources with the same field names
	var relevantConfigs []sigma.Config
	for _, c := range configs {
		for _, v := range c.Logsources {
			if (v.Logsource.Product == r.Logsource.Product || v.Rewrite.Product == r.Logsource.Product) && (v.Logsource.Category == r.Logsource.Category || v.Rewrite.Category == r.Logsource.Category) {
				relevantConfigs = append(relevantConfigs, c)
			}
		}
	}

	if len(relevantConfigs) == 0 {
		return errNoLogSources, nil
	}

	rule := evaluator.ForRule(r, evaluator.WithConfig(relevantConfigs...), evaluator.WithPlaceholderExpander(func(ctx context.Context, placeholderName string) ([]string, error) {
		// TODO: allow test-writers to supply placeholder values
		return nil, nil
	}))
	pass := true
	var failures []string

	for _, tc := range testCases {
		shouldMatch := true
		if tc.Match != nil { // by default, test cases match
			shouldMatch = *tc.Match
		}
		result, _ := rule.Matches(context.Background(), tc.Event)
		switch {
		case shouldMatch && !result.Match:
			pass = false
			failures = append(failures, fmt.Sprintf("%v should have matched", tc.Event))
		case !shouldMatch && result.Match:
			pass = false
			failures = append(failures, fmt.Sprintf("%v shouldn't have matched", tc.Event))
		}
	}
	if pass {
		return nil, nil
	}
	return errFailedTests, failures
}

func getTestCases(path string) ([]TestCase, error) {
	testFile, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var testCases []TestCase
	decoder := yaml.NewDecoder(testFile)
	for {
		testCase := TestCase{}
		err = decoder.Decode(&testCase)
		if err != nil {
			break
		}
		testCases = append(testCases, testCase)
	}
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("error parsing test cases: %w", err)
	}

	// If there's a trailing end of document marker ("---") then there's an empty final test case we need to remove
	if testCases[len(testCases)-1].Event == nil {
		testCases = testCases[:len(testCases)-1]
	}

	return testCases, nil
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
