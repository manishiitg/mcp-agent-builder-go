import { useContext } from 'react';
import { EventModeContext } from './EventContext';

export const useEventMode = () => {
  const context = useContext(EventModeContext);
  if (context === undefined) {
    throw new Error('useEventMode must be used within an EventModeProvider');
  }
  return context;
}; 