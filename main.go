package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type contact struct {
	JID   string `json:"JID"`
	Phone string `json:"Phone"`
	Name  string `json:"Name"`
	Alias string `json:"Alias"`
}

type wacliResult struct {
	Success bool      `json:"success"`
	Data    []contact `json:"data"`
	Error   *string   `json:"error"`
}

type message struct {
	ChatJID   string `json:"ChatJID"`
	ChatName  string `json:"ChatName"`
	MsgID     string `json:"MsgID"`
	SenderJID string `json:"SenderJID"`
	Timestamp string `json:"Timestamp"`
	FromMe    bool   `json:"FromMe"`
	Text      string `json:"Text"`
	MediaType string `json:"MediaType"`
}

type messagesData struct {
	Messages []message `json:"messages"`
}

type wacliMessagesResult struct {
	Success bool         `json:"success"`
	Data    messagesData `json:"data"`
	Error   *string      `json:"error"`
}

func runWacli(args ...string) ([]byte, error) {
	args = append(args, "--json")
	cmd := exec.Command("wacli", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("wacli %s failed: %s\n%s", strings.Join(args, " "), err, string(out))
	}
	return out, nil
}

func searchContacts(query string) ([]contact, error) {
	out, err := runWacli("contacts", "search", query)
	if err != nil {
		return nil, err
	}
	var result wacliResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("failed to parse contacts response: %s", err)
	}
	if result.Error != nil {
		return nil, fmt.Errorf("wacli error: %s", *result.Error)
	}
	if result.Data == nil {
		return []contact{}, nil
	}
	return result.Data, nil
}

var nonDigit = regexp.MustCompile(`\D`)

// resolveJID converts a user input (JID, phone number, or contact name) to a JID.
// Returns the JID and an optional user-facing message (for ambiguous matches).
func resolveJID(input string) (string, string, error) {
	// Already a JID
	if strings.Contains(input, "@") {
		return input, "", nil
	}

	// Check if it looks like a phone number (strip non-digits; if most chars were digits, treat as phone)
	digits := nonDigit.ReplaceAllString(input, "")
	if len(digits) >= 7 {
		return digits + "@s.whatsapp.net", "", nil
	}

	// Otherwise treat as a contact name search
	contacts, err := searchContacts(input)
	if err != nil {
		return "", "", fmt.Errorf("failed to search contacts: %s", err)
	}
	if len(contacts) == 0 {
		return "", "", fmt.Errorf("no contact found for '%s'", input)
	}
	if len(contacts) > 1 {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Multiple contacts match '%s'. Please specify which one:\n\n", input))
		for _, c := range contacts {
			name := c.Name
			if c.Alias != "" {
				name += " (" + c.Alias + ")"
			}
			sb.WriteString(fmt.Sprintf("- %s | JID: %s\n", name, c.JID))
		}
		return "", sb.String(), nil
	}
	return contacts[0].JID, "", nil
}

func handleSearchContacts(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, ok := request.GetArguments()["query"].(string)
	if !ok || query == "" {
		return mcp.NewToolResultError("query parameter is required"), nil
	}

	contacts, err := searchContacts(query)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if len(contacts) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No contacts found for '%s'", query)), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d contact(s):\n\n", len(contacts)))
	for _, c := range contacts {
		name := c.Name
		if c.Alias != "" {
			name += " (" + c.Alias + ")"
		}
		sb.WriteString(fmt.Sprintf("- %s | JID: %s | Phone: %s\n", name, c.JID, c.Phone))
	}
	return mcp.NewToolResultText(sb.String()), nil
}

func handleSendText(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	to, ok := args["to"].(string)
	if !ok || to == "" {
		return mcp.NewToolResultError("to parameter is required"), nil
	}
	message, ok := args["message"].(string)
	if !ok || message == "" {
		return mcp.NewToolResultError("message parameter is required"), nil
	}

	jid, ambiguous, err := resolveJID(to)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if ambiguous != "" {
		return mcp.NewToolResultText(ambiguous), nil
	}

	out, err := runWacli("send", "text", "--to", jid, "--message", message)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(out)), nil
}

func handleReadMessages(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	to, ok := args["from"].(string)
	if !ok || to == "" {
		return mcp.NewToolResultError("from parameter is required"), nil
	}

	limit := 10
	if l, ok := args["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}

	jid, ambiguous, err := resolveJID(to)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if ambiguous != "" {
		return mcp.NewToolResultText(ambiguous), nil
	}

	out, err := runWacli("messages", "list", "--chat", jid, "--limit", fmt.Sprintf("%d", limit))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var result wacliMessagesResult
	if err := json.Unmarshal(out, &result); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to parse messages response: %s", err)), nil
	}
	if result.Error != nil {
		return mcp.NewToolResultError(fmt.Sprintf("wacli error: %s", *result.Error)), nil
	}
	if len(result.Data.Messages) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No messages found for '%s'", to)), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Last %d message(s) with %s:\n\n", len(result.Data.Messages), to))
	for _, m := range result.Data.Messages {
		direction := "Them"
		if m.FromMe {
			direction = "You"
		}
		text := m.Text
		if text == "" && m.MediaType != "" {
			text = fmt.Sprintf("[%s]", m.MediaType)
		}
		sb.WriteString(fmt.Sprintf("[%s] %s: %s\n", m.Timestamp, direction, text))
	}
	return mcp.NewToolResultText(sb.String()), nil
}

func main() {
	s := server.NewMCPServer(
		"wacli-mcp",
		"1.0.0",
	)

	s.AddTool(
		mcp.NewTool("whatsapp_search_contacts",
			mcp.WithDescription("Search WhatsApp contacts by name"),
			mcp.WithString("query",
				mcp.Description("Name to search for"),
				mcp.Required(),
			),
		),
		handleSearchContacts,
	)

	s.AddTool(
		mcp.NewTool("whatsapp_send_text",
			mcp.WithDescription("Send a text message on WhatsApp. Accepts a contact name (auto-resolves to JID) or a JID directly."),
			mcp.WithString("to",
				mcp.Description("Contact name or JID (e.g. 'Ankit Agarwal' or '16502086463@s.whatsapp.net')"),
				mcp.Required(),
			),
			mcp.WithString("message",
				mcp.Description("Message text to send"),
				mcp.Required(),
			),
		),
		handleSendText,
	)

	s.AddTool(
		mcp.NewTool("whatsapp_read_messages",
			mcp.WithDescription("Read recent messages from a WhatsApp chat. Accepts a contact name (auto-resolves to JID) or a JID directly."),
			mcp.WithString("from",
				mcp.Description("Contact name or JID (e.g. 'Ankit Agarwal' or '16502086463@s.whatsapp.net')"),
				mcp.Required(),
			),
			mcp.WithNumber("limit",
				mcp.Description("Number of recent messages to read (default: 10)"),
			),
		),
		handleReadMessages,
	)

	stdio := server.NewStdioServer(s)
	if err := stdio.Listen(context.Background(), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %s\n", err)
		os.Exit(1)
	}
}
