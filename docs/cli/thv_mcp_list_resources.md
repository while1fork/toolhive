---
title: thv mcp list resources
hide_title: true
description: Reference for ToolHive CLI command `thv mcp list resources`
last_update:
  author: autogenerated
slug: thv_mcp_list_resources
mdx:
  format: md
---

## thv mcp list resources

List available resources from MCP server

### Synopsis

List all resources available from the specified MCP server.

```
thv mcp list resources [flags]
```

### Options

```
      --format string      Output format (json or text) (default "text")
  -h, --help               help for resources
      --server string      MCP server URL (required)
      --timeout duration   Connection timeout (default 30s)
      --transport string   Transport type (auto, sse, streamable-http) (default "auto")
```

### Options inherited from parent commands

```
      --debug   Enable debug mode
```

### SEE ALSO

* [thv mcp list](thv_mcp_list.md)	 - List MCP server capabilities

