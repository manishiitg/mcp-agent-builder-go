import React from 'react'

interface ReActExplanationProps {
  agentMode: 'simple' | 'ReAct' | 'orchestrator' | 'workflow'
}

export const ReActExplanation: React.FC<ReActExplanationProps> = ({ agentMode }) => {
  if (agentMode !== 'ReAct') {
    return null
  }

  return (
    <div className="flex items-center justify-center py-12">
      <div className="text-center max-w-2xl">
        {/* Main Icon */}
        <div className="w-20 h-20 mx-auto mb-6 bg-primary/10 rounded-full flex items-center justify-center">
          <svg className="w-10 h-10 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z" />
          </svg>
        </div>

        {/* Title */}
        <h3 className="text-xl font-semibold text-foreground mb-4">
          🧠 ReAct Reasoning Agent
        </h3>

        {/* Description */}
        <p className="text-sm text-muted-foreground mb-6">
          The ReAct agent uses the "reasoning and acting" framework to combine chain-of-thought reasoning with external tool use. <strong className="text-foreground">Step-by-step reasoning</strong> with iterative thought-action-observation loops.
        </p>

        {/* ReAct Loop Visualization */}
        <div className="bg-card border border-border rounded-lg p-6 mb-6">
          <h4 className="font-medium text-card-foreground mb-4 text-sm">ReAct Framework Loop:</h4>
          <div className="flex items-center justify-center space-x-4 text-xs">
            {/* Thought */}
            <div className="flex flex-col items-center">
              <div className="w-12 h-12 bg-blue-100 dark:bg-blue-900/20 rounded-full flex items-center justify-center mb-2">
                <span className="text-blue-600 dark:text-blue-400 font-semibold">💭</span>
              </div>
              <span className="text-muted-foreground font-medium">Thought</span>
              <span className="text-muted-foreground text-xs">Reasoning</span>
            </div>

            {/* Arrow */}
            <div className="text-muted-foreground">
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
              </svg>
            </div>

            {/* Action */}
            <div className="flex flex-col items-center">
              <div className="w-12 h-12 bg-green-100 dark:bg-green-900/20 rounded-full flex items-center justify-center mb-2">
                <span className="text-green-600 dark:text-green-400 font-semibold">⚡</span>
              </div>
              <span className="text-muted-foreground font-medium">Action</span>
              <span className="text-muted-foreground text-xs">Tool Use</span>
            </div>

            {/* Arrow */}
            <div className="text-muted-foreground">
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
              </svg>
            </div>

            {/* Observation */}
            <div className="flex flex-col items-center">
              <div className="w-12 h-12 bg-purple-100 dark:bg-purple-900/20 rounded-full flex items-center justify-center mb-2">
                <span className="text-purple-600 dark:text-purple-400 font-semibold">👁️</span>
              </div>
              <span className="text-muted-foreground font-medium">Observation</span>
              <span className="text-muted-foreground text-xs">Result</span>
            </div>
          </div>
          
          {/* Loop Arrow */}
          <div className="flex justify-center mt-4">
            <div className="text-muted-foreground">
              <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
            </div>
          </div>
        </div>

        {/* Key Features */}
        <div className="bg-muted border border-border rounded-lg p-4 mb-4">
          <h4 className="font-medium text-foreground mb-3 text-sm">Key Features:</h4>
          <div className="grid grid-cols-2 gap-2 text-xs text-muted-foreground">
            <div className="flex items-center gap-1">
              <span>🔄</span>
              <span>Iterative Reasoning</span>
            </div>
            <div className="flex items-center gap-1">
              <span>🛠️</span>
              <span>Tool Integration</span>
            </div>
            <div className="flex items-center gap-1">
              <span>📝</span>
              <span>Explainable Process</span>
            </div>
            <div className="flex items-center gap-1">
              <span>🎯</span>
              <span>Adaptive Planning</span>
            </div>
            <div className="flex items-center gap-1">
              <span>✅</span>
              <span>Reduced Hallucinations</span>
            </div>
            <div className="flex items-center gap-1">
              <span>🧠</span>
              <span>Chain-of-Thought</span>
            </div>
          </div>
        </div>

        {/* Usage Instructions */}
        <div className="flex items-center justify-center gap-2 text-xs text-muted-foreground">
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          <span>Ask complex questions that require step-by-step reasoning and tool usage</span>
        </div>

        {/* IBM Blog Link */}
        <div className="mt-4">
          <a 
            href="https://www.ibm.com/think/topics/react-agent" 
            target="_blank" 
            rel="noopener noreferrer"
            className="inline-flex items-center gap-2 text-xs text-primary hover:text-primary/80 transition-colors"
          >
            <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
            </svg>
            <span>Learn more about ReAct agents on IBM Think</span>
          </a>
        </div>
      </div>
    </div>
  )
}
