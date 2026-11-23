package module

import "strings"

const maxSessionTitleLen = 60

// SessionModule displays the active conversation title (e.g. set via /rename),
// falling back to the conversation/session UUID when no title is present.
type SessionModule struct{}

func NewSessionModule() *SessionModule              { return &SessionModule{} }
func (m *SessionModule) Name() string               { return "session" }
func (m *SessionModule) Collect(ctx *Context) error { return nil }

func (m *SessionModule) Vars(ctx *Context) map[string]string {
	if ctx == nil || ctx.Stdin == nil {
		return nil
	}

	title := sanitizeSingleLine(ctx.Stdin.ConversationTitle, maxSessionTitleLen)
	id := strings.TrimSpace(ctx.Stdin.ConversationID)
	if id == "" {
		id = strings.TrimSpace(ctx.Stdin.SessionID)
	}

	session := title
	if session == "" {
		session = id
	}
	if session == "" {
		return nil
	}

	shortID := id
	if r := []rune(shortID); len(r) > 8 {
		shortID = string(r[:8])
	}

	return map[string]string{
		"session":  session,
		"title":    title,
		"id":       id,
		"short_id": shortID,
	}
}

func (m *SessionModule) DefaultFormat() string {
	return "{session}"
}

// sanitizeSingleLine collapses newlines and tabs to spaces and truncates to maxLen runes.
func sanitizeSingleLine(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.Join(strings.Fields(s), " ")
	return truncateRunes(s, maxLen)
}
