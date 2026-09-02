/**
 * One-line descriptions for the bundled connectors.
 *
 * The MCP tool list carries per-*tool* descriptions but nothing at the server
 * level, so the directory cards would otherwise have an empty second line.
 * These live here for the same reason `brandMarks` does: they are presentation
 * copy, not configuration, and a server missing an entry degrades gracefully.
 *
 * Keyed by the server name exactly as it appears in `mcp_servers_clean.json`.
 * Keep each one short enough to sit on two lines in a card.
 */
const CONNECTOR_DESCRIPTIONS: Record<string, string> = {
  Notion: 'Search, update, and create pages across your workspace',
  Linear: 'Manage issues, projects, and team cycles',
  Sentry: 'Investigate errors and performance issues in your projects',
  Canva: 'Search, create, autofill, and export Canva designs',
  Airtable: 'Query and update records across your Airtable bases',
  PostHog: 'Explore product analytics, funnels, and session data',
  Grafana: 'Query dashboards, metrics, and alert rules',
  Honeycomb: 'Query traces and debug production behaviour',
  MongoDB: 'Explore collections and run queries against your clusters',
  Apify: 'Run scrapers and automation actors on the Apify platform',
  WorkOS: 'Manage organizations, users, and SSO connections',
  Resend: 'Send transactional email and inspect delivery logs',
  Paddle: 'Review subscriptions, transactions, and customer billing',
  Port: 'Query your service catalog and developer portal',
  Indeed: 'Search job listings and manage employer postings',
  Morningstar: 'Look up investment research, funds, and market data',
  'Parallel Search MCP': 'Run web searches built for agent workflows',
  'Hugging Face': 'Search models, datasets, and Spaces on the Hub',
  'Cloudflare Docs': 'Search Cloudflare product documentation',
  'AWS Knowledge': 'Search AWS docs and best-practice guidance',
  Wolfram: 'Compute answers, math, and curated factual data',
  Exa: 'Search the web with embedding-based retrieval',
  Browserbase: 'Drive a hosted headless browser session',
  Cortex: 'Query your internal service catalog and scorecards',
}

/**
 * Copy for a connector card's second line. Custom servers a user adds
 * themselves have no entry, so they fall back to a neutral label rather than
 * leaving the card looking unfinished.
 */
export function descriptionFor(serverName: string): string {
  return CONNECTOR_DESCRIPTIONS[serverName] ?? 'Custom MCP server'
}
