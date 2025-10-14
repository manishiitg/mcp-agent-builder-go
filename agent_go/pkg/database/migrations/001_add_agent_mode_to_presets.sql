-- Migration 001: Add agent_mode column to preset_queries table
-- This migration adds agent mode selection to presets for better user experience

-- Add agent_mode column to preset_queries table
-- Default to 'ReAct' for backward compatibility with existing presets
ALTER TABLE preset_queries ADD COLUMN agent_mode TEXT DEFAULT 'ReAct';

-- Update existing presets to have a default agent mode
-- This ensures all existing presets have a valid agent_mode value
UPDATE preset_queries 
SET agent_mode = 'ReAct' 
WHERE agent_mode IS NULL;

-- Create index for better performance when filtering by agent mode
CREATE INDEX IF NOT EXISTS idx_preset_queries_agent_mode ON preset_queries(agent_mode);
