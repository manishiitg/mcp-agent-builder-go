package testing

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	virtualtools "github.com/manishiitg/coding-agent-loop/agent_go/cmd/server/virtual-tools"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var searchWebLLMProvidersTestCmd = &cobra.Command{
	Use:   "search-web-llm-providers",
	Short: "Test search_web_llm across every supported search provider",
	Long: `Test the search_web_llm path across every hosted MCP provider.

This command calls the real search_web_llm executor directly, so it exercises
	hosted MCP search and validates that the returned response is usable.`,
	RunE: runSearchWebLLMProvidersTest,
}

func runSearchWebLLMProvidersTest(cmd *cobra.Command, args []string) error {
	loadTestingEnvFiles()

	logFile := viper.GetString("log-file")
	logLevel := viper.GetString("log-level")
	InitTestLogger(logFile, logLevel)

	workspaceURL := strings.TrimSpace(viper.GetString("search-web-llm-providers.workspace-url"))
	if workspaceURL == "" {
		workspaceURL = strings.TrimSpace(os.Getenv("WORKSPACE_API_URL"))
	}
	if workspaceURL == "" {
		workspaceURL = "http://127.0.0.1:8081"
	}
	if err := os.Setenv("WORKSPACE_API_URL", workspaceURL); err != nil {
		return fmt.Errorf("failed to set WORKSPACE_API_URL: %w", err)
	}

	query := strings.TrimSpace(viper.GetString("search-web-llm-providers.query"))
	if query == "" {
		query = `Use web search to find https://example.com and answer with one concise sentence that includes the word "example".`
	}
	expectAny := parseCSVList(viper.GetString("search-web-llm-providers.expect-any"))
	if len(expectAny) == 0 {
		expectAny = []string{"example"}
	}

	timeoutValue := strings.TrimSpace(viper.GetString("search-web-llm-providers.provider-timeout"))
	providerTimeout, err := time.ParseDuration(timeoutValue)
	if err != nil {
		return fmt.Errorf("invalid --provider-timeout %q: %w", timeoutValue, err)
	}

	providers := parseCSVList(viper.GetString("search-web-llm-providers.providers"))
	if len(providers) == 0 {
		providers = []string{"parallel", "exa", "firecrawl"}
	}

	executor := virtualtools.CreateSearchWebLLMProviderTestExecutor(workspaceURL)
	if executor == nil {
		return fmt.Errorf("search_web_llm executor is not available")
	}

	failFast := viper.GetBool("search-web-llm-providers.fail-fast")

	var passed, skipped, failed int
	var failures []string
	fmt.Printf("Testing search_web_llm providers\n")
	fmt.Printf("Workspace URL: %s\n", workspaceURL)
	fmt.Printf("Query: %s\n\n", query)

	for _, provider := range providers {
		provider = strings.ToLower(strings.TrimSpace(provider))
		if provider == "" {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), providerTimeout)
		args := map[string]any{
			"query":    query,
			"provider": provider,
		}

		start := time.Now()
		result, err := executor(ctx, args)
		cancel()

		if err != nil {
			failed++
			msg := fmt.Sprintf("%s failed after %s: %v", provider, time.Since(start).Round(time.Millisecond), err)
			failures = append(failures, msg)
			fmt.Printf("[FAIL] %s\n", msg)
			if failFast {
				break
			}
			continue
		}

		if strings.TrimSpace(result) == "" {
			failed++
			msg := fmt.Sprintf("%s returned an empty response", provider)
			failures = append(failures, msg)
			fmt.Printf("[FAIL] %s\n", msg)
			if failFast {
				break
			}
			continue
		}
		if len(expectAny) > 0 && !responseContainsAny(result, expectAny) {
			failed++
			msg := fmt.Sprintf("%s response did not contain any expected marker %v: %s", provider, expectAny, oneLinePreview(result, 220))
			failures = append(failures, msg)
			fmt.Printf("[FAIL] %s\n", msg)
			if failFast {
				break
			}
			continue
		}

		passed++
		fmt.Printf("[PASS] %s in %s: %s\n", provider, time.Since(start).Round(time.Millisecond), oneLinePreview(result, 220))
	}

	fmt.Printf("\nSummary: %d passed, %d skipped, %d failed\n", passed, skipped, failed)
	if len(failures) > 0 {
		sort.Strings(failures)
		for _, failure := range failures {
			fmt.Printf("- %s\n", failure)
		}
		return fmt.Errorf("search_web_llm provider matrix had %d failure(s)", failed)
	}
	return nil
}

func init() {
	searchWebLLMProvidersTestCmd.Flags().String("workspace-url", "", "Workspace API URL (default: WORKSPACE_API_URL or http://127.0.0.1:8081)")
	searchWebLLMProvidersTestCmd.Flags().String("query", "", "Web search query to run")
	searchWebLLMProvidersTestCmd.Flags().String("expect-any", "", "Comma-separated response markers; at least one must appear. Defaults to example")
	searchWebLLMProvidersTestCmd.Flags().String("providers", "", "Comma-separated hosted MCP providers to test (default: parallel,exa,firecrawl)")
	searchWebLLMProvidersTestCmd.Flags().String("provider-timeout", "2m", "Timeout per provider")
	searchWebLLMProvidersTestCmd.Flags().Bool("require-all", false, "Reserved for compatibility; all selected MCP providers are always attempted")
	searchWebLLMProvidersTestCmd.Flags().Bool("fail-fast", false, "Stop after the first provider failure")

	viper.BindPFlag("search-web-llm-providers.workspace-url", searchWebLLMProvidersTestCmd.Flags().Lookup("workspace-url"))
	viper.BindPFlag("search-web-llm-providers.query", searchWebLLMProvidersTestCmd.Flags().Lookup("query"))
	viper.BindPFlag("search-web-llm-providers.expect-any", searchWebLLMProvidersTestCmd.Flags().Lookup("expect-any"))
	viper.BindPFlag("search-web-llm-providers.providers", searchWebLLMProvidersTestCmd.Flags().Lookup("providers"))
	viper.BindPFlag("search-web-llm-providers.provider-timeout", searchWebLLMProvidersTestCmd.Flags().Lookup("provider-timeout"))
	viper.BindPFlag("search-web-llm-providers.require-all", searchWebLLMProvidersTestCmd.Flags().Lookup("require-all"))
	viper.BindPFlag("search-web-llm-providers.fail-fast", searchWebLLMProvidersTestCmd.Flags().Lookup("fail-fast"))
}
