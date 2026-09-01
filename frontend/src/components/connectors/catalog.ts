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
  Lovable: 'Build and deploy apps and websites from a prompt',
  Supabase: 'Manage and query your Postgres databases',
  Vercel: 'Inspect deployments, logs, and project settings',
  Atlassian: 'Work with Jira issues and Confluence pages',
  Asana: 'Track tasks, projects, and team workload',
  Intercom: 'Search conversations, contacts, and help articles',
  ClickUp: 'Manage tasks, docs, and project spaces',
  Mixpanel: 'Query product analytics events and funnels',
  Stripe: 'Review payments, customers, and subscriptions',
  Ramp: 'Review card spend, bills, and reimbursements',
  Zapier: 'Trigger and run automations across your apps',
  Square: 'Manage payments, orders, and catalog items',
  PayPal: 'Review transactions, invoices, and payouts',
  Webflow: 'Manage sites, CMS collections, and publishing',
  Wix: 'Manage site content, stores, and bookings',
  'monday.com': 'Track boards, items, and team workflows',
  Neon: 'Query and branch serverless Postgres databases',
  Netlify: 'Inspect sites, deploys, and build logs',
  Cloudflare: 'Manage Workers, KV, R2, and DNS bindings',
  Datadog: 'Query metrics, monitors, logs, and traces',
  Plaid: 'Inspect linked accounts and financial data',
  Prisma: 'Manage Postgres databases and schema changes',
  Sanity: 'Query and edit structured content documents',
  Amplitude: 'Explore product analytics and user journeys',
  Miro: 'Work with boards, frames, and sticky notes',
  Railway: 'Deploy services and inspect build logs',
  Contentful: 'Query and edit structured content entries',
  Upstash: 'Manage serverless Redis and Kafka data',
  CircleCI: 'Inspect pipelines, workflows, and job logs',
  Loops: 'Send product email and manage contacts',
  Shortcut: 'Track stories, epics, and iterations',
}

/**
 * Copy for a connector card's second line. Custom servers a user adds
 * themselves have no entry, so they fall back to a neutral label rather than
 * leaving the card looking unfinished.
 */
export function descriptionFor(serverName: string): string {
  return CONNECTOR_DESCRIPTIONS[serverName] ?? 'Custom MCP server'
}

/**
 * Section a connector is filed under in the directory, mirroring how the
 * upstream plugin directories group the same services. Presentation copy like
 * the descriptions above, not configuration.
 *
 * A server with no entry falls into "Other", which is where user-added custom
 * servers land too, so nothing drops out of the directory for want of a
 * category.
 */
const CONNECTOR_CATEGORIES: Record<string, string> = {
  Sentry: 'Developer Tools',
  Grafana: 'Developer Tools',
  Honeycomb: 'Developer Tools',
  MongoDB: 'Developer Tools',
  Supabase: 'Developer Tools',
  Vercel: 'Developer Tools',
  WorkOS: 'Developer Tools',
  Port: 'Developer Tools',
  Apify: 'Developer Tools',
  Resend: 'Developer Tools',
  Lovable: 'Developer Tools',
  Notion: 'Productivity',
  Linear: 'Productivity',
  Asana: 'Productivity',
  ClickUp: 'Productivity',
  Atlassian: 'Productivity',
  Stripe: 'Business & Operations',
  Paddle: 'Business & Operations',
  Ramp: 'Business & Operations',
  Intercom: 'Business & Operations',
  Indeed: 'Business & Operations',
  PostHog: 'Data & Analytics',
  Mixpanel: 'Data & Analytics',
  Airtable: 'Data & Analytics',
  Canva: 'Creativity',
  Neon: 'Developer Tools',
  Netlify: 'Developer Tools',
  Cloudflare: 'Developer Tools',
  Datadog: 'Developer Tools',
  Prisma: 'Developer Tools',
  Zapier: 'Developer Tools',
  Miro: 'Productivity',
  'monday.com': 'Productivity',
  Square: 'Business & Operations',
  PayPal: 'Business & Operations',
  Plaid: 'Business & Operations',
  Amplitude: 'Data & Analytics',
  Webflow: 'Creativity',
  Wix: 'Creativity',
  Sanity: 'Creativity',
  Railway: 'Developer Tools',
  Upstash: 'Developer Tools',
  CircleCI: 'Developer Tools',
  Contentful: 'Creativity',
  Loops: 'Business & Operations',
  Shortcut: 'Productivity',
}

/** Fallback section for connectors with no category, including custom ones. */
export const OTHER_CATEGORY = 'Other'

/**
 * Display order for the directory's sections. A category outside this list
 * sorts after the known ones rather than disappearing.
 */
export const CATEGORY_ORDER: string[] = [
  'Developer Tools',
  'Productivity',
  'Business & Operations',
  'Data & Analytics',
  'Creativity',
  OTHER_CATEGORY,
]

/** The directory section a connector belongs to. */
export function categoryFor(serverName: string): string {
  return CONNECTOR_CATEGORIES[serverName] ?? OTHER_CATEGORY
}
