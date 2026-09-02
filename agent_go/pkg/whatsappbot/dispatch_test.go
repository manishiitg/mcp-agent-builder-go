package whatsappbot

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

type recordingHandler struct {
	messages    []*Message
	activated   []string
	deactivated []string
	unknown     []string
	commands    []string
}

func (h *recordingHandler) HandleMessage(_ context.Context, msg *Message) {
	h.messages = append(h.messages, msg)
}

func (h *recordingHandler) HandleCommand(_ context.Context, msg *Message) bool {
	if strings.HasPrefix(msg.Text, "@list") {
		h.commands = append(h.commands, msg.Text)
		return true
	}
	return false
}

func (h *recordingHandler) RouteActivated(_ context.Context, _ *Message, route *Route, continuing bool) {
	suffix := ""
	if continuing {
		suffix = "+"
	}
	h.activated = append(h.activated, route.Key+suffix)
}

func (h *recordingHandler) RouteDeactivated(_ context.Context, _ *Message, key string) {
	h.deactivated = append(h.deactivated, key)
}

func (h *recordingHandler) RouteUnknown(_ context.Context, _ *Message, token string) {
	h.unknown = append(h.unknown, token)
}

func newTestConnector(h Handler, router Router) *Connector {
	return New(Config{Handler: h, Router: router, Access: SelfChatOnly{}})
}

func inbound(text string, media bool) *Message {
	return &Message{Chat: types.NewJID("15551234567", types.DefaultUserServer), SelfChat: true, RawText: text, Text: text, HasMedia: media}
}

func slugRouter(known ...string) Router {
	return RouterFunc(func(_ context.Context, _ string, token string) *Route {
		for _, k := range known {
			if k == token {
				return &Route{Key: k, Value: k}
			}
		}
		return nil
	})
}

func TestDispatchPrefixMentionRouting(t *testing.T) {
	h := &recordingHandler{}
	c := newTestConnector(h, slugRouter("invoices"))

	c.dispatch(context.Background(), inbound("@invoices", false))
	if len(h.messages) != 0 || len(h.activated) != 1 || h.activated[0] != "invoices" {
		t.Fatalf("bare mention should only activate: msgs=%d activated=%v", len(h.messages), h.activated)
	}

	c.dispatch(context.Background(), inbound("hello there", false))
	if len(h.messages) != 1 || h.messages[0].Route == nil || h.messages[0].Route.Key != "invoices" || h.messages[0].Switched {
		t.Fatalf("remembered route should apply to plain text: %+v", h.messages)
	}

	c.dispatch(context.Background(), inbound("@invoices summarise this", false))
	last := h.messages[len(h.messages)-1]
	if last.Text != "summarise this" || !last.Switched || h.activated[len(h.activated)-1] != "invoices+" {
		t.Fatalf("mention with text should strip the token and continue: text=%q switched=%v activated=%v", last.Text, last.Switched, h.activated)
	}

	c.dispatch(context.Background(), inbound("@invoices off", false))
	if len(h.deactivated) != 1 || c.routeStore().Active(inbound("", false).Chat.String()) != "" {
		t.Fatalf("deactivate word should clear the route: %v", h.deactivated)
	}

	c.dispatch(context.Background(), inbound("@nope", false))
	if len(h.unknown) != 1 || h.unknown[0] != "nope" {
		t.Fatalf("bare unknown mention should be reported: %v", h.unknown)
	}
	before := len(h.messages)
	c.dispatch(context.Background(), inbound("@nope but keep going", false))
	if len(h.messages) != before+1 || h.messages[before].Text != "@nope but keep going" || h.messages[before].Route != nil {
		t.Fatalf("unknown mention with text should pass through unchanged: %+v", h.messages[before])
	}
}

func TestDispatchCommandsRunBeforeRouting(t *testing.T) {
	h := &recordingHandler{}
	c := newTestConnector(h, slugRouter("list"))
	c.dispatch(context.Background(), inbound("@list", false))
	if len(h.commands) != 1 || len(h.activated) != 0 || len(h.messages) != 0 {
		t.Fatalf("command should be consumed before routing: cmds=%v activated=%v", h.commands, h.activated)
	}
}

func TestDispatchAccessPolicyDropsForeignChats(t *testing.T) {
	h := &recordingHandler{}
	c := newTestConnector(h, nil)
	msg := inbound("hi", false)
	msg.SelfChat = false
	c.dispatch(context.Background(), msg)
	if len(h.messages) != 0 {
		t.Fatal("non-self chat must be dropped by SelfChatOnly")
	}
}

type anywhereRouter struct{ re *regexp.Regexp }

func (r anywhereRouter) Resolve(_ context.Context, _ string, token string) *Route {
	return &Route{Key: token}
}

func (r anywhereRouter) MatchMention(text string) (string, string, bool) {
	m := r.re.FindStringSubmatch(text)
	if m == nil {
		return "", text, false
	}
	rest := strings.Join(strings.Fields(r.re.ReplaceAllString(text, " ")), " ")
	return strings.ToLower(m[1]), rest, true
}

type fixedStore struct{ active string }

func (s *fixedStore) Active(string) string   { return s.active }
func (s *fixedStore) Activate(_, key string) { s.active = key }
func (s *fixedStore) Deactivate(string)      { s.active = "parent" }

func TestDispatchCustomMatcherAndStore(t *testing.T) {
	h := &recordingHandler{}
	store := &fixedStore{active: "parent"}
	c := New(Config{Handler: h, Router: anywhereRouter{regexp.MustCompile(`(?i)@(child|parent)\b`)}, Routes: store})

	c.dispatch(context.Background(), inbound("photos of homework @child please", true))
	if store.active != "child" || len(h.messages) != 1 || h.messages[0].Text != "photos of homework please" || h.messages[0].Route.Key != "child" {
		t.Fatalf("anywhere mention should switch and continue: store=%q msgs=%+v", store.active, h.messages)
	}
	c.dispatch(context.Background(), inbound("plain follow-up", false))
	if h.messages[1].Route.Key != "child" || h.messages[1].Switched {
		t.Fatalf("store default route should be applied: %+v", h.messages[1])
	}
}

func TestParseMention(t *testing.T) {
	token, rest, ok := ParseMention("  @Invoices   run it ")
	if !ok || token != "invoices" || rest != "run it" {
		t.Fatalf("got %q %q %v", token, rest, ok)
	}
	if _, _, ok := ParseMention("no mention"); ok {
		t.Fatal("plain text must not parse as mention")
	}
}
