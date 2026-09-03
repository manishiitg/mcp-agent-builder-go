import { Sparkles } from 'lucide-react'
import type { CommandDefinition } from '../../../src/commands/types'
import type { QuickCommand } from '../stores/types'

// SparkQuill's product commands (product.yaml `commands:`) as slash commands
// for the shared composer. Same contract as Video Studio's: the prompt is
// submitted on the parent's behalf.
export function toProductCommandDefinitions(commands: QuickCommand[]): CommandDefinition[] {
  return commands.map((c) => ({
    command: c.label.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, ''),
    description: c.label,
    icon: <Sparkles className="w-4 h-4" />,
    modes: ['multi-agent'],
    source: 'product',
    execute: (ctx) => { ctx.onSubmit(c.message) },
  }))
}
