---
title: thv logs prune
hide_title: true
description: Reference for ToolHive CLI command `thv logs prune`
last_update:
  author: autogenerated
slug: thv_logs_prune
mdx:
  format: md
---

## thv logs prune

Delete log files from servers not currently managed by ToolHive

### Synopsis

Delete log files from servers that are not currently managed by ToolHive (running or stopped).
This helps clean up old log files that accumulate over time from removed servers.

```
thv logs prune [flags]
```

### Options

```
  -h, --help   help for prune
```

### Options inherited from parent commands

```
      --debug   Enable debug mode
```

### SEE ALSO

* [thv logs](thv_logs.md)	 - Output the logs of an MCP server or manage log files

