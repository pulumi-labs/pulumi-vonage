package provider

import (
	"bytes"
	"context"
	"encoding/pem"
	"fmt"
	"strings"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// ---------------------------------------------------------------------------
// Application resource (Vonage Application API v2)
// ---------------------------------------------------------------------------

// Application manages a Vonage application: the container that holds the
// voice, messages, RTC, verify, VBC and meetings capabilities and their
// webhooks, plus the public/private key pair used to sign JWTs.
type Application struct{}

// Webhook is a callback URL plus HTTP method.
type Webhook struct {
	Address    string  `pulumi:"address"`
	HTTPMethod *string `pulumi:"httpMethod,optional"`
}

func (w *Webhook) Annotate(a infer.Annotator) {
	a.Describe(&w.Address, "The URL Vonage calls.")
	a.Describe(&w.HTTPMethod, "HTTP method Vonage uses: GET or POST. Messages, verify and meetings webhooks only accept POST.")
}

// VoiceWebhook is a Webhook with the voice-specific read timeouts.
type VoiceWebhook struct {
	Address        string  `pulumi:"address"`
	HTTPMethod     *string `pulumi:"httpMethod,optional"`
	ConnectTimeout *int    `pulumi:"connectTimeout,optional"`
	SocketTimeout  *int    `pulumi:"socketTimeout,optional"`
}

func (w *VoiceWebhook) Annotate(a infer.Annotator) {
	a.Describe(&w.Address, "The URL Vonage calls.")
	a.Describe(&w.HTTPMethod, "HTTP method Vonage uses: GET or POST.")
	a.Describe(&w.ConnectTimeout, "Milliseconds Vonage waits to connect to the webhook (300-1000).")
	a.Describe(&w.SocketTimeout, "Milliseconds Vonage waits for the webhook response to be read (1000-10000).")
}

// VoiceCapability configures the voice capability.
type VoiceCapability struct {
	AnswerURL          *VoiceWebhook `pulumi:"answerUrl,optional"`
	FallbackAnswerURL  *VoiceWebhook `pulumi:"fallbackAnswerUrl,optional"`
	EventURL           *VoiceWebhook `pulumi:"eventUrl,optional"`
	SignedCallbacks    *bool         `pulumi:"signedCallbacks,optional"`
	ConversationsTTL   *int          `pulumi:"conversationsTtl,optional"`
	LegPersistenceTime *int          `pulumi:"legPersistenceTime,optional"`
	Region             *string       `pulumi:"region,optional"`
}

func (v *VoiceCapability) Annotate(a infer.Annotator) {
	a.Describe(&v.AnswerURL, "Webhook Vonage calls to fetch the NCCO that controls an inbound or answered call.")
	a.Describe(&v.FallbackAnswerURL, "Webhook used if answerUrl is offline or returns an HTTP error.")
	a.Describe(&v.EventURL, "Webhook that receives call state events.")
	a.Describe(&v.SignedCallbacks, "Whether Vonage signs webhook requests (server default true).")
	a.Describe(&v.ConversationsTTL, "Hours a named conversation stays active after creation (1-9000).")
	a.Describe(&v.LegPersistenceTime, "Days call legs persist (1-31).")
	a.Describe(&v.Region, "Region that handles inbound, programmable SIP and SIP Connect calls: "+
		"na-east, na-west, eu-east, eu-west, apac-sng or apac-australia.")
}

// MessagesCapability configures the messages capability.
type MessagesCapability struct {
	InboundURL               *Webhook `pulumi:"inboundUrl,optional"`
	StatusURL                *Webhook `pulumi:"statusUrl,optional"`
	Version                  *string  `pulumi:"version,optional"`
	AuthenticateInboundMedia *bool    `pulumi:"authenticateInboundMedia,optional"`
}

func (m *MessagesCapability) Annotate(a infer.Annotator) {
	a.Describe(&m.InboundURL, "Webhook that receives inbound messages (POST only).")
	a.Describe(&m.StatusURL, "Webhook that receives message status events (POST only).")
	a.Describe(&m.Version, "Messages API version, for example v1 or v0.1.")
	a.Describe(&m.AuthenticateInboundMedia, "Whether inbound media URLs require authentication.")
}

// RtcCapability configures the RTC (in-app voice and chat) capability.
type RtcCapability struct {
	EventURL        *Webhook `pulumi:"eventUrl,optional"`
	SignedCallbacks *bool    `pulumi:"signedCallbacks,optional"`
}

func (r *RtcCapability) Annotate(a infer.Annotator) {
	a.Describe(&r.EventURL, "Webhook that receives RTC events.")
	a.Describe(&r.SignedCallbacks, "Whether Vonage signs webhook requests.")
}

// VerifyCapability configures the Verify v2 capability.
type VerifyCapability struct {
	StatusURL *Webhook `pulumi:"statusUrl,optional"`
	Version   *string  `pulumi:"version,optional"`
}

func (v *VerifyCapability) Annotate(a infer.Annotator) {
	a.Describe(&v.StatusURL, "Webhook that receives Verify request status events (POST only).")
	a.Describe(&v.Version, "Verify API version, for example v2.")
}

// MeetingsCapability configures the meetings capability.
type MeetingsCapability struct {
	RecordingChanged *Webhook `pulumi:"recordingChanged,optional"`
	RoomChanged      *Webhook `pulumi:"roomChanged,optional"`
	SessionChanged   *Webhook `pulumi:"sessionChanged,optional"`
}

func (m *MeetingsCapability) Annotate(a infer.Annotator) {
	a.Describe(&m.RecordingChanged, "Webhook that receives recording status changes (POST only).")
	a.Describe(&m.RoomChanged, "Webhook that receives room changes (POST only).")
	a.Describe(&m.SessionChanged, "Webhook that receives session changes (POST only).")
}

// ApplicationArgs are the inputs of an Application.
type ApplicationArgs struct {
	Name      string              `pulumi:"name"`
	Voice     *VoiceCapability    `pulumi:"voice,optional"`
	Messages  *MessagesCapability `pulumi:"messages,optional"`
	Rtc       *RtcCapability      `pulumi:"rtc,optional"`
	Verify    *VerifyCapability   `pulumi:"verify,optional"`
	Meetings  *MeetingsCapability `pulumi:"meetings,optional"`
	Vbc       *bool               `pulumi:"vbc,optional"`
	ImproveAi *bool               `pulumi:"improveAi,optional"`
	PublicKey *string             `pulumi:"publicKey,optional"`
}

func (a *ApplicationArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Name, "Application name (shown in the Vonage dashboard).")
	an.Describe(&a.Voice, "Voice capability. Omit to leave voice disabled.")
	an.Describe(&a.Messages, "Messages capability. Omit to leave messages disabled.")
	an.Describe(&a.Rtc, "RTC capability. Omit to leave RTC disabled.")
	an.Describe(&a.Verify, "Verify v2 capability. Omit to leave verify disabled.")
	an.Describe(&a.Meetings, "Meetings capability. Omit to leave meetings disabled.")
	an.Describe(&a.Vbc, "Set true to enable the (empty) vbc capability for zero-rated VBC calls.")
	an.Describe(&a.ImproveAi, "Whether Vonage may use traffic to improve its AI models (privacy.improve_ai).")
	an.Describe(&a.PublicKey, "Optional PEM public key. When omitted Vonage generates a key pair on creation and "+
		"the private key is returned once as privateKey. When set, no private key is generated.")
}

// ApplicationState is the persisted state of an Application.
type ApplicationState struct {
	ApplicationArgs

	// ApplicationID is the server-assigned application ID.
	ApplicationID string `pulumi:"applicationId"`
	// ApplicationPublicKey is the public key registered with the application.
	ApplicationPublicKey string `pulumi:"applicationPublicKey"`
	// PrivateKey is returned by Vonage only when the application is created without a publicKey.
	PrivateKey *string `pulumi:"privateKey,optional" provider:"secret"`
}

func (s *ApplicationState) Annotate(a infer.Annotator) {
	a.Describe(&s.ApplicationID, "The Vonage application ID (UUID). Use it as app_id for numbers and as the JWT application_id claim.")
	a.Describe(&s.ApplicationPublicKey, "The PEM public key registered with the application.")
	a.Describe(&s.PrivateKey, "The PEM private key generated by Vonage. Vonage returns it exactly once, on creation, and "+
		"never again; it is stored as a secret in Pulumi state. Unset when publicKey was supplied.")
}

func (r *Application) Annotate(a infer.Annotator) {
	a.Describe(&r, "A Vonage application (Application API v2): capabilities, webhooks and signing keys. "+
		"Update replaces the whole configuration, so every capability you want must be declared in the program; capabilities this provider does not model (video, network_apis) are preserved.")
}

var _ infer.CustomResource[ApplicationArgs, ApplicationState] = (*Application)(nil)
var _ infer.CustomCheck[ApplicationArgs] = (*Application)(nil)
var _ infer.CustomRead[ApplicationArgs, ApplicationState] = (*Application)(nil)
var _ infer.CustomUpdate[ApplicationArgs, ApplicationState] = (*Application)(nil)
var _ infer.CustomDelete[ApplicationState] = (*Application)(nil)

func checkMethod(failures *[]p.CheckFailure, prop string, method *string, postOnly bool) {
	if method == nil {
		return
	}
	m := strings.ToUpper(*method)
	if postOnly && m != "POST" {
		*failures = append(*failures, p.CheckFailure{Property: prop, Reason: "httpMethod must be POST for this webhook"})
		return
	}
	if m != "GET" && m != "POST" {
		*failures = append(*failures, p.CheckFailure{Property: prop, Reason: "httpMethod must be GET or POST"})
	}
}

func checkRange(failures *[]p.CheckFailure, prop string, v *int, lo, hi int) {
	if v != nil && (*v < lo || *v > hi) {
		*failures = append(*failures, p.CheckFailure{Property: prop, Reason: fmt.Sprintf("must be between %d and %d", lo, hi)})
	}
}

func checkWebhook(failures *[]p.CheckFailure, prop string, w *Webhook, postOnly bool) {
	if w == nil {
		return
	}
	if strings.TrimSpace(w.Address) == "" {
		*failures = append(*failures, p.CheckFailure{Property: prop + ".address", Reason: "address must not be empty"})
	}
	checkMethod(failures, prop+".httpMethod", w.HTTPMethod, postOnly)
}

func checkVoiceWebhook(failures *[]p.CheckFailure, prop string, w *VoiceWebhook) {
	if w == nil {
		return
	}
	if strings.TrimSpace(w.Address) == "" {
		*failures = append(*failures, p.CheckFailure{Property: prop + ".address", Reason: "address must not be empty"})
	}
	checkMethod(failures, prop+".httpMethod", w.HTTPMethod, false)
	checkRange(failures, prop+".connectTimeout", w.ConnectTimeout, 300, 1000)
	checkRange(failures, prop+".socketTimeout", w.SocketTimeout, 1000, 10000)
}

var validRegions = map[string]bool{
	"na-east": true, "na-west": true, "eu-east": true, "eu-west": true, "apac-sng": true, "apac-australia": true,
}

// Check validates ranges and enums that the Vonage API would otherwise reject
// only at apply time.
func (r *Application) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[ApplicationArgs], error) {
	args, failures, err := infer.DefaultCheck[ApplicationArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[ApplicationArgs]{Inputs: args, Failures: failures}, err
	}
	if strings.TrimSpace(args.Name) == "" {
		failures = append(failures, p.CheckFailure{Property: "name", Reason: "name must not be empty"})
	}
	if v := args.Voice; v != nil {
		checkVoiceWebhook(&failures, "voice.answerUrl", v.AnswerURL)
		checkVoiceWebhook(&failures, "voice.fallbackAnswerUrl", v.FallbackAnswerURL)
		checkVoiceWebhook(&failures, "voice.eventUrl", v.EventURL)
		checkRange(&failures, "voice.conversationsTtl", v.ConversationsTTL, 1, 9000)
		checkRange(&failures, "voice.legPersistenceTime", v.LegPersistenceTime, 1, 31)
		if v.Region != nil && !validRegions[*v.Region] {
			failures = append(failures, p.CheckFailure{Property: "voice.region",
				Reason: "must be one of na-east, na-west, eu-east, eu-west, apac-sng, apac-australia"})
		}
	}
	if m := args.Messages; m != nil {
		checkWebhook(&failures, "messages.inboundUrl", m.InboundURL, true)
		checkWebhook(&failures, "messages.statusUrl", m.StatusURL, true)
	}
	if rt := args.Rtc; rt != nil {
		checkWebhook(&failures, "rtc.eventUrl", rt.EventURL, false)
	}
	if v := args.Verify; v != nil {
		checkWebhook(&failures, "verify.statusUrl", v.StatusURL, true)
	}
	if m := args.Meetings; m != nil {
		checkWebhook(&failures, "meetings.recordingChanged", m.RecordingChanged, true)
		checkWebhook(&failures, "meetings.roomChanged", m.RoomChanged, true)
		checkWebhook(&failures, "meetings.sessionChanged", m.SessionChanged, true)
	}
	return infer.CheckResponse[ApplicationArgs]{Inputs: args, Failures: failures}, nil
}

// --- args <-> wire conversion ---

func toWireHook(w *Webhook) *wireWebhook {
	if w == nil {
		return nil
	}
	out := &wireWebhook{Address: w.Address}
	if w.HTTPMethod != nil {
		out.HTTPMethod = strings.ToUpper(*w.HTTPMethod)
	}
	return out
}

func toWireVoiceHook(w *VoiceWebhook) *wireWebhook {
	if w == nil {
		return nil
	}
	out := &wireWebhook{Address: w.Address, ConnectTimeout: w.ConnectTimeout, SocketTimeout: w.SocketTimeout}
	if w.HTTPMethod != nil {
		out.HTTPMethod = strings.ToUpper(*w.HTTPMethod)
	}
	return out
}

func toWireApplication(a ApplicationArgs) *wireApplication {
	w := &wireApplication{Name: a.Name}
	caps := &wireCapabilities{}
	hasCap := false
	if v := a.Voice; v != nil {
		wv := &wireVoice{SignedCallbacks: v.SignedCallbacks, ConversationsTTL: v.ConversationsTTL,
			LegPersistenceTime: v.LegPersistenceTime, Region: v.Region}
		if v.AnswerURL != nil || v.FallbackAnswerURL != nil || v.EventURL != nil {
			wv.Webhooks = &struct {
				AnswerURL         *wireWebhook `json:"answer_url,omitempty"`
				FallbackAnswerURL *wireWebhook `json:"fallback_answer_url,omitempty"`
				EventURL          *wireWebhook `json:"event_url,omitempty"`
			}{toWireVoiceHook(v.AnswerURL), toWireVoiceHook(v.FallbackAnswerURL), toWireVoiceHook(v.EventURL)}
		}
		caps.Voice, hasCap = wv, true
	}
	if m := a.Messages; m != nil {
		wm := &wireMessages{Version: m.Version, AuthenticateInboundMedia: m.AuthenticateInboundMedia}
		if m.InboundURL != nil || m.StatusURL != nil {
			wm.Webhooks = &struct {
				InboundURL *wireWebhook `json:"inbound_url,omitempty"`
				StatusURL  *wireWebhook `json:"status_url,omitempty"`
			}{toWireHook(m.InboundURL), toWireHook(m.StatusURL)}
		}
		caps.Messages, hasCap = wm, true
	}
	if rt := a.Rtc; rt != nil {
		wr := &wireRTC{SignedCallbacks: rt.SignedCallbacks}
		if rt.EventURL != nil {
			wr.Webhooks = &struct {
				EventURL *wireWebhook `json:"event_url,omitempty"`
			}{toWireHook(rt.EventURL)}
		}
		caps.RTC, hasCap = wr, true
	}
	if v := a.Verify; v != nil {
		wv := &wireVerify{Version: v.Version}
		if v.StatusURL != nil {
			wv.Webhooks = &struct {
				StatusURL *wireWebhook `json:"status_url,omitempty"`
			}{toWireHook(v.StatusURL)}
		}
		caps.Verify, hasCap = wv, true
	}
	if m := a.Meetings; m != nil {
		wm := &wireMeetings{}
		if m.RecordingChanged != nil || m.RoomChanged != nil || m.SessionChanged != nil {
			wm.Webhooks = &struct {
				RecordingChanged *wireWebhook `json:"recording_changed,omitempty"`
				RoomChanged      *wireWebhook `json:"room_changed,omitempty"`
				SessionChanged   *wireWebhook `json:"session_changed,omitempty"`
			}{toWireHook(m.RecordingChanged), toWireHook(m.RoomChanged), toWireHook(m.SessionChanged)}
		}
		caps.Meetings, hasCap = wm, true
	}
	if a.Vbc != nil && *a.Vbc {
		caps.VBC, hasCap = &struct{}{}, true
	}
	if hasCap {
		w.Capabilities = caps
	}
	if a.ImproveAi != nil {
		w.Privacy = &wirePrivacy{ImproveAI: a.ImproveAi}
	}
	if a.PublicKey != nil && *a.PublicKey != "" {
		w.Keys = &wireKeys{PublicKey: *a.PublicKey}
	}
	return w
}

func strPtr(s string) *string { return &s }

// observeHook reconciles a user-declared webhook with what the server holds.
// Optional fields the user left unset are not populated from server defaults,
// so an unchanged program never shows a diff.
func observeHook(user *Webhook, obs *wireWebhook) *Webhook {
	if obs == nil {
		return nil
	}
	out := &Webhook{Address: obs.Address}
	if user == nil || user.HTTPMethod != nil {
		if obs.HTTPMethod != "" {
			out.HTTPMethod = strPtr(obs.HTTPMethod)
		}
	}
	if user != nil && user.HTTPMethod != nil && out.HTTPMethod != nil &&
		strings.EqualFold(*user.HTTPMethod, *out.HTTPMethod) {
		out.HTTPMethod = user.HTTPMethod
	}
	return out
}

func observeVoiceHook(user *VoiceWebhook, obs *wireWebhook) *VoiceWebhook {
	if obs == nil {
		return nil
	}
	out := &VoiceWebhook{Address: obs.Address}
	if user == nil {
		if obs.HTTPMethod != "" {
			out.HTTPMethod = strPtr(obs.HTTPMethod)
		}
		out.ConnectTimeout, out.SocketTimeout = obs.ConnectTimeout, obs.SocketTimeout
		return out
	}
	if user.HTTPMethod != nil && obs.HTTPMethod != "" {
		out.HTTPMethod = strPtr(obs.HTTPMethod)
		if strings.EqualFold(*user.HTTPMethod, obs.HTTPMethod) {
			out.HTTPMethod = user.HTTPMethod
		}
	}
	if user.ConnectTimeout != nil {
		out.ConnectTimeout = obs.ConnectTimeout
	}
	if user.SocketTimeout != nil {
		out.SocketTimeout = obs.SocketTimeout
	}
	return out
}

// samePublicKey reports whether two PEM public keys carry the same key material, so
// CRLF line endings, re-wrapped base64 or surrounding whitespace are not drift. Values
// that do not decode as PEM fall back to a whitespace-insensitive comparison.
func samePublicKey(a, b string) bool {
	pa, _ := pem.Decode([]byte(strings.TrimSpace(a)))
	pb, _ := pem.Decode([]byte(strings.TrimSpace(b)))
	if pa != nil && pb != nil {
		return pa.Type == pb.Type && bytes.Equal(pa.Bytes, pb.Bytes)
	}
	return strings.Join(strings.Fields(a), "") == strings.Join(strings.Fields(b), "")
}

// observeApplication projects the server view onto the args shape, using the
// previous inputs to decide which optional scalars are user-managed.
func observeApplication(prev ApplicationArgs, w *wireApplication) ApplicationArgs {
	out := ApplicationArgs{Name: w.Name, PublicKey: prev.PublicKey}
	// A declared publicKey is refreshed from the server so a key rotated outside
	// Pulumi shows up as drift. PEM whitespace differences are not drift. When the
	// key was not declared, the registered key is exposed only through the
	// applicationPublicKey output.
	if prev.PublicKey != nil && w.Keys != nil && w.Keys.PublicKey != "" &&
		!samePublicKey(*prev.PublicKey, w.Keys.PublicKey) {
		out.PublicKey = strPtr(w.Keys.PublicKey)
	}
	caps := w.Capabilities
	if caps == nil {
		caps = &wireCapabilities{}
	}
	if cv := caps.Voice; cv != nil {
		pv := prev.Voice
		if pv == nil {
			pv = &VoiceCapability{}
		}
		v := &VoiceCapability{}
		if cv.Webhooks != nil {
			v.AnswerURL = observeVoiceHook(pv.AnswerURL, cv.Webhooks.AnswerURL)
			v.FallbackAnswerURL = observeVoiceHook(pv.FallbackAnswerURL, cv.Webhooks.FallbackAnswerURL)
			v.EventURL = observeVoiceHook(pv.EventURL, cv.Webhooks.EventURL)
		}
		if prev.Voice == nil || pv.SignedCallbacks != nil {
			v.SignedCallbacks = cv.SignedCallbacks
		}
		if prev.Voice == nil || pv.ConversationsTTL != nil {
			v.ConversationsTTL = cv.ConversationsTTL
		}
		if prev.Voice == nil || pv.LegPersistenceTime != nil {
			v.LegPersistenceTime = cv.LegPersistenceTime
		}
		if prev.Voice == nil || pv.Region != nil {
			v.Region = cv.Region
		}
		out.Voice = v
	}
	if cm := caps.Messages; cm != nil {
		pm := prev.Messages
		if pm == nil {
			pm = &MessagesCapability{}
		}
		m := &MessagesCapability{}
		if cm.Webhooks != nil {
			m.InboundURL = observeHook(pm.InboundURL, cm.Webhooks.InboundURL)
			m.StatusURL = observeHook(pm.StatusURL, cm.Webhooks.StatusURL)
		}
		// messages.version is write-only: the Application API does not return it on
		// GET, so when the server omits it keep what was declared rather than
		// reading it back as removed.
		switch {
		case cm.Version != nil && (prev.Messages == nil || pm.Version != nil):
			m.Version = cm.Version
		case cm.Version == nil:
			m.Version = pm.Version
		}
		if prev.Messages == nil || pm.AuthenticateInboundMedia != nil {
			m.AuthenticateInboundMedia = cm.AuthenticateInboundMedia
		}
		out.Messages = m
	}
	if cr := caps.RTC; cr != nil {
		pr := prev.Rtc
		if pr == nil {
			pr = &RtcCapability{}
		}
		r := &RtcCapability{}
		if cr.Webhooks != nil {
			r.EventURL = observeHook(pr.EventURL, cr.Webhooks.EventURL)
		}
		if prev.Rtc == nil || pr.SignedCallbacks != nil {
			r.SignedCallbacks = cr.SignedCallbacks
		}
		out.Rtc = r
	}
	if cv := caps.Verify; cv != nil {
		pv := prev.Verify
		if pv == nil {
			pv = &VerifyCapability{}
		}
		v := &VerifyCapability{}
		if cv.Webhooks != nil {
			v.StatusURL = observeHook(pv.StatusURL, cv.Webhooks.StatusURL)
		}
		switch {
		case cv.Version != nil && (prev.Verify == nil || pv.Version != nil):
			v.Version = cv.Version
		case cv.Version == nil:
			v.Version = pv.Version
		}
		out.Verify = v
	}
	if cm := caps.Meetings; cm != nil {
		pm := prev.Meetings
		if pm == nil {
			pm = &MeetingsCapability{}
		}
		m := &MeetingsCapability{}
		if cm.Webhooks != nil {
			m.RecordingChanged = observeHook(pm.RecordingChanged, cm.Webhooks.RecordingChanged)
			m.RoomChanged = observeHook(pm.RoomChanged, cm.Webhooks.RoomChanged)
			m.SessionChanged = observeHook(pm.SessionChanged, cm.Webhooks.SessionChanged)
		}
		out.Meetings = m
	}
	if caps.VBC != nil {
		t := true
		out.Vbc = &t
	} else if prev.Vbc != nil && !*prev.Vbc {
		out.Vbc = prev.Vbc
	}
	if w.Privacy != nil && w.Privacy.ImproveAI != nil && (prev.ImproveAi != nil || prev.Name == "") {
		out.ImproveAi = w.Privacy.ImproveAI
	} else {
		out.ImproveAi = prev.ImproveAi
	}
	return out
}

func stateFromWire(args ApplicationArgs, w *wireApplication, priorPrivate *string) ApplicationState {
	st := ApplicationState{ApplicationArgs: args, ApplicationID: w.ID, PrivateKey: priorPrivate}
	if w.Keys != nil {
		st.ApplicationPublicKey = w.Keys.PublicKey
		if w.Keys.PrivateKey != "" {
			k := w.Keys.PrivateKey
			st.PrivateKey = &k
		}
	}
	return st
}

func (r *Application) Create(ctx context.Context, req infer.CreateRequest[ApplicationArgs]) (infer.CreateResponse[ApplicationState], error) {
	if req.DryRun {
		return infer.CreateResponse[ApplicationState]{ID: req.Name, Output: ApplicationState{ApplicationArgs: req.Inputs}}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.CreateResponse[ApplicationState]{}, err
	}
	w, err := c.CreateApplication(ctx, toWireApplication(req.Inputs))
	if err != nil {
		return infer.CreateResponse[ApplicationState]{}, fmt.Errorf("create application %q: %w", req.Inputs.Name, err)
	}
	if w.ID == "" {
		return infer.CreateResponse[ApplicationState]{}, fmt.Errorf("create application %q: response did not include an id", req.Inputs.Name)
	}
	return infer.CreateResponse[ApplicationState]{ID: w.ID, Output: stateFromWire(req.Inputs, w, nil)}, nil
}

func (r *Application) Read(ctx context.Context, req infer.ReadRequest[ApplicationArgs, ApplicationState]) (infer.ReadResponse[ApplicationArgs, ApplicationState], error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.ReadResponse[ApplicationArgs, ApplicationState]{}, err
	}
	w, err := c.GetApplication(ctx, req.ID)
	if err != nil {
		return infer.ReadResponse[ApplicationArgs, ApplicationState]{}, fmt.Errorf("read application %s: %w", req.ID, err)
	}
	if w == nil {
		return infer.ReadResponse[ApplicationArgs, ApplicationState]{}, nil
	}
	args := observeApplication(req.Inputs, w)
	// The private key is create-only; keep what state already holds.
	return infer.ReadResponse[ApplicationArgs, ApplicationState]{
		ID: req.ID, Inputs: args, State: stateFromWire(args, w, req.State.PrivateKey),
	}, nil
}

func (r *Application) Update(ctx context.Context, req infer.UpdateRequest[ApplicationArgs, ApplicationState]) (infer.UpdateResponse[ApplicationState], error) {
	if req.DryRun {
		st := ApplicationState{ApplicationArgs: req.Inputs, ApplicationID: req.State.ApplicationID,
			ApplicationPublicKey: req.State.ApplicationPublicKey, PrivateKey: req.State.PrivateKey}
		return infer.UpdateResponse[ApplicationState]{Output: st}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.UpdateResponse[ApplicationState]{}, err
	}
	body := toWireApplication(req.Inputs)
	// The update is a full replace. Preserve capabilities this provider does not
	// model (video, network_apis, ...) that were configured outside Pulumi.
	cur, err := c.GetApplication(ctx, req.ID)
	if err != nil {
		return infer.UpdateResponse[ApplicationState]{}, fmt.Errorf("read application %s before update: %w", req.ID, err)
	}
	if cur == nil {
		return infer.UpdateResponse[ApplicationState]{}, fmt.Errorf("update application %s: it no longer exists; run pulumi refresh", req.ID)
	}
	if cur.Capabilities != nil && len(cur.Capabilities.Extra) > 0 {
		if body.Capabilities == nil {
			body.Capabilities = &wireCapabilities{}
		}
		body.Capabilities.Extra = cur.Capabilities.Extra
	}
	w, err := c.UpdateApplication(ctx, req.ID, body)
	if err != nil {
		return infer.UpdateResponse[ApplicationState]{}, fmt.Errorf("update application %s: %w", req.ID, err)
	}
	if w.ID == "" {
		w.ID = req.ID
	}
	// ApplicationPublicKey comes from the update response (identity-adjacent
	// output refresh); only the create-only private key is carried from state.
	st := stateFromWire(req.Inputs, w, req.State.PrivateKey)
	if w.Keys == nil {
		st.ApplicationPublicKey = req.State.ApplicationPublicKey
	}
	return infer.UpdateResponse[ApplicationState]{Output: st}, nil
}

func (r *Application) Delete(ctx context.Context, req infer.DeleteRequest[ApplicationState]) (infer.DeleteResponse, error) {
	c, err := clientFor(ctx)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	if err := c.DeleteApplication(ctx, req.ID); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("delete application %s: %w", req.ID, err)
	}
	return infer.DeleteResponse{}, nil
}
