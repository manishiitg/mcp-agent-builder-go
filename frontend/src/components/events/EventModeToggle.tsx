import React from 'react';
import { Button } from '../ui/Button';
import { useEventMode } from './useEventMode';
import { useAppStore } from '../../stores/useAppStore';
import { Eye, EyeOff, Settings, Workflow } from 'lucide-react';

export const EventModeToggle: React.FC = () => {
  const { mode, setMode } = useEventMode();
  const { agentMode } = useAppStore();

  const cycleMode = () => {
    switch (mode) {
      case 'basic':
        setMode('advanced');
        break;
      case 'advanced':
        // Context-aware cycling based on agent mode
        if (agentMode === 'workflow') {
          setMode('workflow');
        } else if (agentMode === 'orchestrator') {
          setMode('orchestrator');
        } else {
          // For simple/ReAct agent modes, cycle back to basic
          setMode('basic');
        }
        break;
      case 'workflow':
        // From workflow, cycle back to basic
        setMode('basic');
        break;
      case 'orchestrator':
        // From Deep Search, cycle back to basic
        setMode('basic');
        break;
      default:
        setMode('basic');
    }
  };

  const getModeDisplay = () => {
    switch (mode) {
      case 'basic':
        return { icon: Eye, label: 'Basic' };
      case 'advanced':
        return { icon: EyeOff, label: 'Advanced' };
      case 'orchestrator':
        return { icon: Settings, label: 'Deep Search' };
      case 'workflow':
        return { icon: Workflow, label: 'Workflow' };
      default:
        return { icon: Eye, label: 'Basic' };
    }
  };

  const { icon: Icon, label } = getModeDisplay();

  return (
    <div className="flex items-center gap-1">
      <span className="text-xs text-gray-600 dark:text-gray-400">
        Event Mode:
      </span>
      <Button
        variant="outline"
        size="sm"
        onClick={cycleMode}
        className="flex items-center gap-1 text-xs h-7 px-2"
      >
        <Icon className="w-3 h-3" />
        {label}
      </Button>
    </div>
  );
}; 