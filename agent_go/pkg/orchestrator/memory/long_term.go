package memory

// LongTermMemory contains requirements for Graph RAG memory operations
type LongTermMemory struct {
}

// NewLongTermMemory creates a new instance for long-term memory management
func NewLongTermMemory() *LongTermMemory {
	return &LongTermMemory{}
}

// GetLongTermMemoryRequirements returns requirements for Graph RAG memory operations only
func (ltm *LongTermMemory) GetLongTermMemoryRequirements() string {
	return `## 🧠 **LONG-TERM MEMORY**

### **📋 Memory Operations**
- **add_memory**: Store important findings, decisions, and insights in knowledge graph
- **search_memory**: Search knowledge graph for relevant past information and context
- **delete_memory**: Delete outdated or incorrect memories from the knowledge graph
- **Use Cases**: Quick facts, key decisions, important findings, context from other agents
- **Best Practices**: 
  - Store concise, factual information
  - Use descriptive titles for easy retrieval
  - Include relevant context and timestamps
  - Store inter-agent coordination details
  - Clean up outdated information to maintain memory quality
  - Delete memories that are no longer accurate or relevant

### **🎯 When to Use Long-Term Memory**
- **Quick Facts**: Important numbers, dates, key findings
- Important data is will act a long term knowledgebase

### **⚠️ Guidelines**
- **Support Role**: Memory operations support your primary task, don't replace it
- **Accuracy**: Verify information before storing
- **Cleanup**: Remove outdated or incorrect memories
- **Context**: Include enough context for future retrieval`
}
