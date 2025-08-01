---
title: thv secret setup
hide_title: true
description: Reference for ToolHive CLI command `thv secret setup`
last_update:
  author: autogenerated
slug: thv_secret_setup
mdx:
  format: md
---

## thv secret setup

Set up secrets provider

### Synopsis

Interactive setup for configuring a secrets provider.

This command guides you through selecting and configuring a secrets provider
for storing and retrieving secrets. The setup process validates your
configuration and ensures the selected provider initializes properly.

Available providers:
  - encrypted: Stores secrets in an encrypted file using AES-256-GCM using the OS keyring
  - 1password: Read-only access to 1Password secrets (requires OP_SERVICE_ACCOUNT_TOKEN environment variable)
  - none: Disables secrets functionality

Run this command before using any other secrets functionality.

```
thv secret setup [flags]
```

### Options

```
  -h, --help   help for setup
```

### Options inherited from parent commands

```
      --debug   Enable debug mode
```

### SEE ALSO

* [thv secret](thv_secret.md)	 - Manage secrets

