import React from 'react'
import { formatUSD } from './helpers'

// One label/value tile in a stage-cost grid.
const StageCostCard: React.FC<{ label: string; value: number; shadow?: boolean }> = ({ label, value, shadow = false }) => (
  <div className={`bg-card border border-border rounded-lg p-3${shadow ? ' shadow-sm' : ''}`}>
    <div className="text-xs text-muted-foreground font-medium mb-1 uppercase tracking-wider">{label}</div>
    <div className="text-lg font-bold text-foreground">{formatUSD(value)}</div>
  </div>
)

export default StageCostCard
