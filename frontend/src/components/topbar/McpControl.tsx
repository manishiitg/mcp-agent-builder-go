import { ServerCog } from 'lucide-react'
import IconPopover from '../ui/IconPopover'
import MCPServersSection from '../sidebar/MCPServersSection'

/** Connectors popover trigger. */
export default function McpControl() {
  return (
    <IconPopover
      icon={<ServerCog className="w-4 h-4" />}
      label="Connectors"
      dataTour="sidebar-mcp-servers"
      dataTestid="tour-sidebar-mcp-servers"
    >
      <MCPServersSection />
    </IconPopover>
  )
}
