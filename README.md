# wacli-mcp

An MCP (Model Context Protocol) server that exposes WhatsApp messaging capabilities to AI assistants like Claude. It wraps the [wacli](https://github.com/nicholascross/wacli) CLI tool and communicates over stdio.

## Prerequisites

- Go 1.25+
- `wacli` CLI installed and authenticated (`wacli auth`)
- `wacli sync` should be running in the background for real-time message reception

## Build

```bash
go build -o wacli-mcp .
```

## Tools

The server exposes three tools:

### `whatsapp_search_contacts`

Search WhatsApp contacts by name.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | yes | Name to search for |

### `whatsapp_send_text`

Send a text message on WhatsApp.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `to` | string | yes | Contact name, phone number, or JID |
| `message` | string | yes | Message text to send |

### `whatsapp_read_messages`

Read recent messages from a WhatsApp chat.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `from` | string | yes | Contact name, phone number, or JID |
| `limit` | number | no | Number of recent messages to read (default: 10) |

## Contact Resolution

All tools that accept a contact identifier support three formats:

- **Contact name** - e.g. `"John Doe"` (searched via `wacli contacts search`)
- **Phone number** - e.g. `"+1 (555) 123-4567"` or `"15551234567"` (stripped to digits and converted to JID)
- **JID** - e.g. `"1234567890@s.whatsapp.net"` (used directly)

If a name search returns multiple matches, the tool returns the list and asks the user to be more specific.

## Claude Desktop Setup

Add the following to your Claude Desktop MCP config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "Whatsapp": {
      "command": "/path/to/wacli-mcp"
    }
  }
}
```

## Syncing Messages

The `whatsapp_read_messages` tool reads from `wacli`'s local database. To ensure messages are up to date:

```bash
# One-time sync
wacli sync --once

# Continuous sync (recommended for real-time message reading)
wacli sync

# Backfill history for a specific chat
wacli history backfill --chat "1234567890@s.whatsapp.net" --count 50
```

## Testing

You can test the server directly via stdin:

```bash
# List tools
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}' | ./wacli-mcp

# Search contacts
echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"whatsapp_search_contacts","arguments":{"query":"John"}}}' | ./wacli-mcp

# Send a message
echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"whatsapp_send_text","arguments":{"to":"John Doe","message":"Hello!"}}}' | ./wacli-mcp

# Read messages
echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"whatsapp_read_messages","arguments":{"from":"+1 (555) 123-4567","limit":5}}}' | ./wacli-mcp
```
