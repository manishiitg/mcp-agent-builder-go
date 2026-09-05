interface NotificationInstructionsProps {
  title: string
  instructions: string
}

export default function NotificationInstructions({ title, instructions }: NotificationInstructionsProps) {
  return (
    <details className="group w-full min-w-0 text-xs">
      <summary className="cursor-pointer rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
        <span className="font-medium text-primary">
          <span className="group-open:hidden">Show full instructions</span>
          <span className="hidden group-open:inline">Show less</span>
          <span className="sr-only"> for {title}</span>
        </span>
        <span className="mt-1 block truncate italic text-muted-foreground group-open:hidden">“{instructions}”</span>
      </summary>
      <p className="mt-2 whitespace-pre-wrap break-words text-muted-foreground [overflow-wrap:anywhere]">{instructions}</p>
    </details>
  )
}
