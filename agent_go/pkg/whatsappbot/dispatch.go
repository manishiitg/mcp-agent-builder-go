package whatsappbot

import (
	"context"
	"strings"
)

// Handler is what a product does with a routed message.
type Handler interface {
	HandleMessage(ctx context.Context, msg *Message)
}

// HandlerFunc adapts a function to Handler.
type HandlerFunc func(ctx context.Context, msg *Message)

// HandleMessage implements Handler.
func (f HandlerFunc) HandleMessage(ctx context.Context, msg *Message) { f(ctx, msg) }

// AccessPolicy decides whether a chat may talk to the bot. A policy may
// answer the sender itself (e.g. a link-code handshake) and return false.
type AccessPolicy interface {
	Allow(ctx context.Context, msg *Message) bool
}

// SelfChatOnly admits only the linked phone's own "message yourself" chat.
type SelfChatOnly struct{}

// Allow implements AccessPolicy.
func (SelfChatOnly) Allow(_ context.Context, msg *Message) bool { return msg.SelfChat }

// Route is a resolved "@token" destination. Value is whatever the product
// attaches (an agent profile, a workflow, ...).
type Route struct {
	Key   string
	Value interface{}
}

// Router resolves mention tokens. Resolve returns nil for unknown tokens.
type Router interface {
	Resolve(ctx context.Context, chat string, token string) *Route
}

// RouterFunc adapts a function to Router.
type RouterFunc func(ctx context.Context, chat string, token string) *Route

// Resolve implements Router.
func (f RouterFunc) Resolve(ctx context.Context, chat string, token string) *Route {
	return f(ctx, chat, token)
}

// MentionMatcher lets a product recognise mentions its own way (for example
// "@child" anywhere in the text). Without it the connector only looks at a
// leading "@token". Implement it on the Router.
type MentionMatcher interface {
	MatchMention(text string) (token, rest string, ok bool)
}

// RouteStore remembers the active route of each chat. Active returns "" for
// none; Deactivate resets the chat to the product's default.
type RouteStore interface {
	Active(chat string) string
	Activate(chat, key string)
	Deactivate(chat string)
}

// CommandHandler runs before routing; a product uses it for control
// commands ("@list", "@status", ...). Return true when the message was consumed.
type CommandHandler interface {
	HandleCommand(ctx context.Context, msg *Message) bool
}

// RouteObserver is told about route changes so the product can acknowledge
// them in its own voice. continuing is true when the message also carries
// text or media that will still be delivered to HandleMessage.
type RouteObserver interface {
	RouteActivated(ctx context.Context, msg *Message, route *Route, continuing bool)
	RouteDeactivated(ctx context.Context, msg *Message, key string)
	RouteUnknown(ctx context.Context, msg *Message, token string)
}

// ParseMention splits a leading "@token rest" mention.
func ParseMention(text string) (token, rest string, ok bool) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "@") {
		return "", text, false
	}
	firstSpace := strings.IndexAny(trimmed, " \t\n")
	if firstSpace < 0 {
		return strings.ToLower(strings.TrimSpace(trimmed[1:])), "", true
	}
	return strings.ToLower(strings.TrimSpace(trimmed[1:firstSpace])), strings.TrimSpace(trimmed[firstSpace+1:]), true
}

// IsDeactivateWord reports whether text is one of the "turn this off" words.
func IsDeactivateWord(text string) bool {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "deactivate", "deactive", "off", "stop":
		return true
	}
	return false
}

func (c *Connector) routeStore() RouteStore {
	if c.cfg.Routes != nil {
		return c.cfg.Routes
	}
	return memoryRoutes{c}
}

type memoryRoutes struct{ c *Connector }

func (m memoryRoutes) Active(chat string) string {
	m.c.routesMu.Lock()
	defer m.c.routesMu.Unlock()
	return m.c.routes[chat]
}

func (m memoryRoutes) Activate(chat, key string) {
	m.c.routesMu.Lock()
	defer m.c.routesMu.Unlock()
	m.c.routes[chat] = key
}

func (m memoryRoutes) Deactivate(chat string) {
	m.c.routesMu.Lock()
	defer m.c.routesMu.Unlock()
	delete(m.c.routes, chat)
}

// dispatch runs the product-independent pipeline: access, commands,
// mention routing, then the handler.
func (c *Connector) dispatch(ctx context.Context, msg *Message) {
	access := c.cfg.Access
	if access == nil {
		access = SelfChatOnly{}
	}
	if !access.Allow(ctx, msg) {
		return
	}
	handler := c.cfg.Handler
	if cmd, ok := handler.(CommandHandler); ok && cmd.HandleCommand(ctx, msg) {
		return
	}
	router := c.cfg.Router
	if router != nil {
		chat := msg.Chat.String()
		store := c.routeStore()
		observer, _ := handler.(RouteObserver)
		matcher, hasMatcher := router.(MentionMatcher)
		var token, rest string
		var mentioned bool
		if hasMatcher {
			token, rest, mentioned = matcher.MatchMention(msg.Text)
		} else {
			token, rest, mentioned = ParseMention(msg.Text)
		}
		if mentioned {
			continuing := rest != "" || msg.HasMedia
			if route := router.Resolve(ctx, chat, token); route != nil {
				if IsDeactivateWord(rest) {
					store.Deactivate(chat)
					if observer != nil {
						observer.RouteDeactivated(ctx, msg, route.Key)
					}
					return
				}
				store.Activate(chat, route.Key)
				msg.Route = route
				msg.Switched = true
				msg.Text = rest
				if observer != nil {
					observer.RouteActivated(ctx, msg, route, continuing)
				}
				if !continuing {
					return
				}
			} else if !continuing {
				if observer != nil {
					observer.RouteUnknown(ctx, msg, token)
				}
				return
			}
		}
		if msg.Route == nil {
			if key := store.Active(chat); key != "" {
				if route := router.Resolve(ctx, chat, key); route != nil {
					msg.Route = route
				} else {
					store.Deactivate(chat)
				}
			}
		}
	}
	handler.HandleMessage(ctx, msg)
}
