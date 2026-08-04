import type { CSSProperties } from 'react'
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter'
import { oneDark, prism } from 'react-syntax-highlighter/dist/esm/styles/prism'

export interface SyntaxHighlightedCodeProps {
  code: string
  language: string
  isDark: boolean
}

export default function SyntaxHighlightedCode({ code, language, isDark }: SyntaxHighlightedCodeProps) {
  return <SyntaxHighlighter
    style={isDark ? (oneDark as Record<string, CSSProperties>) : (prism as Record<string, CSSProperties>)}
    language={language}
    PreTag="div"
    customStyle={{
      margin: 0,
      padding: '1rem',
      borderRadius: '.55rem',
      fontSize: '.82rem',
      lineHeight: '1.6',
      overflowX: 'auto',
      maxWidth: '100%',
      width: '100%',
      boxSizing: 'border-box',
      background: isDark ? '#1f2937' : '#f8f6f1',
    }}
    codeTagProps={{ style: { display: 'block', overflowX: 'auto', maxWidth: '100%' } }}
  >
    {code}
  </SyntaxHighlighter>
}
