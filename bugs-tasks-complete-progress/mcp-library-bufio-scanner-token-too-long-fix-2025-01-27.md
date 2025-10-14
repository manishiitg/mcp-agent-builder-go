# MCP Library bufio.Scanner Token Too Long Fix - 2025-01-27

## 🐛 **Bug Summary**
**Issue**: `bufio.Scanner: token too long` error occurs when MCP stdio servers output large content (e.g., browser automation tools like Playwright)
**Status**: 🔴 **CRITICAL** - Blocking browser automation functionality
**Priority**: **HIGH** - Affects core MCP functionality
**Date**: 2025-01-27

## 📋 **Problem Description**

### **Error Details**
```
2025/09/30 08:09:52 ERROR: Error reading from stdout: bufio.Scanner: token too long
```

### **Root Cause**
The `github.com/mark3labs/mcp-go` library's `NewStdioMCPClient()` function internally uses `bufio.Scanner` with the default 64KB buffer limit. When browser automation tools (like Playwright) generate large outputs (HTML content, screenshots, JSON responses), they exceed this limit and cause the error.

### **Affected Components**
- **MCP Stdio Transport**: `client/transport/stdio.go` in mcp-go library
- **Browser Automation Tools**: Playwright, Puppeteer, Selenium
- **Large Output Scenarios**: HTML content, screenshots, large JSON responses
- **Our Code**: `agent_go/pkg/mcpclient/stdio_manager.go` (not the source of the issue)

## 🔍 **Investigation Results**

### **MCP Library Analysis**
- **Current Version**: `github.com/mark3labs/mcp-go v0.41.0`
- **PR #464**: [Fix exists](https://github.com/mark3labs/mcp-go/pull/464/files) but not in our version
- **Fix Date**: August 24, 2025 (merged)
- **Fix Details**: Switches from `bufio.Reader` to `bufio.Scanner` with proper error handling

### **Fix Implementation in PR #464**
```go
// Before (causing the issue)
stdout: bufio.NewReader(input),
line, err := c.stdout.ReadString('\n')

// After (the fix)
stdout: bufio.NewScanner(input),
if !c.stdout.Scan() {
    err := c.stdout.Err()
    // Better error handling
}
line := c.stdout.Text()
```

### **Our Code Status**
- **Our Implementation**: Already has proper buffer size configuration (1MB) in `testNPXCommand()`
- **Error Source**: Not from our code, but from MCP library's internal implementation
- **Version Issue**: The fix exists but is not available in v0.41.0

## 🎯 **Solution Options**

### **Option 1: Fork and Patch MCP Library** ⭐ **RECOMMENDED**
**Approach**: Create our own fork of `github.com/mark3labs/mcp-go` and apply the fix
**Benefits**:
- Immediate fix for our use case
- Full control over the implementation
- Can customize buffer sizes for our specific needs
- No dependency on upstream library updates

**Implementation Steps**:
1. Fork `github.com/mark3labs/mcp-go` to our organization
2. Apply the fix from PR #464
3. Add additional buffer size configuration options
4. Update our go.mod to use our forked version
5. Test with browser automation tools

### **Option 2: Wait for Upstream Fix**
**Approach**: Wait for the official release with the fix
**Risks**:
- Unknown timeline for release
- May not include all optimizations we need
- Blocks current development

### **Option 3: Implement Workaround**
**Approach**: Add error handling and retry logic in our code
**Limitations**:
- Doesn't fix the root cause
- May not work for all scenarios
- Adds complexity to our codebase

## 🛠️ **Recommended Implementation Plan**

### **Phase 1: Fork Creation**
1. **Fork Repository**: Create fork of `github.com/mark3labs/mcp-go`
2. **Apply Fix**: Implement the changes from PR #464
3. **Enhance Buffer Configuration**: Add configurable buffer sizes
4. **Add Logging**: Improve error logging and debugging

### **Phase 2: Integration**
1. **Update Dependencies**: Modify go.mod to use our fork
2. **Test Integration**: Verify fix works with browser automation
3. **Performance Testing**: Ensure no performance regression
4. **Documentation**: Update documentation with new buffer options

### **Phase 3: Monitoring**
1. **Monitor Upstream**: Watch for official releases
2. **Contribute Back**: Submit improvements to upstream
3. **Maintain Fork**: Keep fork updated with upstream changes

## 📊 **Technical Details**

### **Buffer Size Requirements**
- **Default Limit**: 64KB (causing the issue)
- **Our Current**: 1MB (in testNPXCommand)
- **Recommended**: 10MB+ for browser automation
- **Configurable**: Should be adjustable per server type

### **Error Scenarios**
- **Browser Screenshots**: Can be several MB
- **HTML Content**: Large pages with embedded content
- **JSON Responses**: Large API responses
- **Log Outputs**: Verbose logging from tools

### **Performance Considerations**
- **Memory Usage**: Larger buffers use more memory
- **Startup Time**: Buffer allocation impact
- **Concurrent Connections**: Multiple large buffers

## 🧪 **Testing Strategy**

### **Test Cases**
1. **Browser Automation**: Playwright with large page content
2. **Screenshot Capture**: Large image outputs
3. **API Responses**: Large JSON responses
4. **Log Streaming**: Verbose tool outputs
5. **Concurrent Usage**: Multiple large outputs simultaneously

### **Success Criteria**
- ✅ No "token too long" errors
- ✅ Large outputs processed correctly
- ✅ No performance regression
- ✅ Proper error handling and logging

## 📝 **Implementation Checklist**

### **Fork Setup**
- [ ] Fork `github.com/mark3labs/mcp-go` repository
- [ ] Clone fork to local development environment
- [ ] Create development branch for our fixes
- [ ] Set up CI/CD for automated testing

### **Fix Implementation**
- [ ] Apply changes from PR #464
- [ ] Add configurable buffer size options
- [ ] Implement proper error handling
- [ ] Add comprehensive logging
- [ ] Write unit tests for new functionality

### **Integration**
- [ ] Update go.mod to use our fork
- [ ] Test with existing MCP servers
- [ ] Verify browser automation tools work
- [ ] Performance testing and optimization
- [ ] Documentation updates

### **Deployment**
- [ ] Deploy to development environment
- [ ] Test with real browser automation scenarios
- [ ] Monitor for any issues
- [ ] Deploy to production environment

## 🔗 **References**

- **MCP Library**: https://github.com/mark3labs/mcp-go
- **Fix PR**: https://github.com/mark3labs/mcp-go/pull/464/files
- **Our Code**: `agent_go/pkg/mcpclient/stdio_manager.go`
- **Error Log**: Terminal selection showing the error
- **Related Issues**: Browser automation tool outputs

## 📈 **Impact Assessment**

### **Current Impact**
- 🔴 **CRITICAL**: Browser automation tools completely broken
- 🔴 **HIGH**: Large output scenarios fail
- 🔴 **MEDIUM**: Error handling needs improvement

### **Post-Fix Impact**
- ✅ **RESOLVED**: Browser automation tools work correctly
- ✅ **IMPROVED**: Large outputs handled gracefully
- ✅ **ENHANCED**: Better error handling and logging

## 🎯 **Next Steps**

1. **Immediate**: Create fork of MCP library
2. **Short-term**: Implement and test the fix
3. **Medium-term**: Integrate with our codebase
4. **Long-term**: Contribute improvements back to upstream

---

**Status**: 🔴 **IN PROGRESS** - Fork creation and fix implementation needed
**Assignee**: Development Team
**Due Date**: TBD
**Priority**: **HIGH** - Blocking browser automation functionality
