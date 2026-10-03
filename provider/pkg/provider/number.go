package provider

import (
	"context"
	"fmt"
	"strings"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi-go-provider/infer"
)

// ---------------------------------------------------------------------------
// Number resource (Vonage Numbers API)
// ---------------------------------------------------------------------------

// Number manages the lifecycle and routing of a Vonage virtual number.
//
// BILLING AND DESTRUCTIVE SEMANTICS. Buying a number is billed and recurring;
// cancelling a number releases it permanently and it may not be possible to
// get the same number back. Because of that, this resource makes both
// decisions explicit:
//
//   - acquisition must be "buy" (purchase the number) or "adopt" (manage a
//     number the account already owns, without purchasing anything).
//   - cancelOnDelete must be set to true or false. Only true cancels the number
//     when the resource is destroyed; false merely stops managing it (the number
//     stays on the account and keeps billing).
type Number struct{}

// NumberArgs are the inputs of a Number.
type NumberArgs struct {
	Country             string  `pulumi:"country" provider:"replaceOnChanges"`
	Msisdn              string  `pulumi:"msisdn" provider:"replaceOnChanges"`
	Acquisition         string  `pulumi:"acquisition" provider:"replaceOnChanges"`
	CancelOnDelete      bool    `pulumi:"cancelOnDelete"`
	AppID               *string `pulumi:"appId,optional"`
	MoHTTPURL           *string `pulumi:"moHttpUrl,optional"`
	VoiceCallbackType   *string `pulumi:"voiceCallbackType,optional"`
	VoiceCallbackValue  *string `pulumi:"voiceCallbackValue,optional"`
	VoiceStatusCallback *string `pulumi:"voiceStatusCallback,optional"`
}

func (a *NumberArgs) Annotate(an infer.Annotator) {
	an.Describe(&a.Country, "Two-letter ISO 3166-1 alpha-2 country code of the number, for example US. Changing it replaces the resource.")
	an.Describe(&a.Msisdn, "The number in international format without a leading plus, for example 14155550100. Changing it replaces the resource.")
	an.Describe(&a.Acquisition, "How the number joins the account: \"buy\" purchases it (BILLED, fails if the account already owns it) "+
		"or \"adopt\" manages a number the account already owns (fails if it does not). Changing it replaces the resource.")
	an.Describe(&a.CancelOnDelete, "Required explicit choice. true: destroying this resource CANCELS (permanently releases) the number. "+
		"false: destroying it only removes it from Pulumi state; the number stays on the account and keeps billing.")
	an.Describe(&a.AppID, "Application ID the number is linked to. Mutually exclusive with moHttpUrl and voiceCallbackType/Value.")
	an.Describe(&a.MoHTTPURL, "Webhook for inbound SMS when the number is not linked to an application.")
	an.Describe(&a.VoiceCallbackType, "Voice routing type when the number is not linked to an application: sip, tel or app.")
	an.Describe(&a.VoiceCallbackValue, "Voice routing target: a SIP URI, a phone number or an application ID, matching voiceCallbackType.")
	an.Describe(&a.VoiceStatusCallback, "Webhook that receives voice call status events for the number.")
}

// NumberState is the persisted state of a Number.
type NumberState struct {
	NumberArgs

	// Type is the number type reported by Vonage (landline, mobile-lvn, landline-toll-free).
	Type string `pulumi:"type"`
	// Features lists the capabilities of the number (SMS, VOICE, MMS).
	Features []string `pulumi:"features"`
	// MessagesCallbackType is the effective inbound messages routing type.
	MessagesCallbackType string `pulumi:"messagesCallbackType"`
	// MessagesCallbackValue is the effective inbound messages routing target.
	MessagesCallbackValue string `pulumi:"messagesCallbackValue"`
	// EffectiveVoiceCallbackType is the voice routing type Vonage currently applies.
	EffectiveVoiceCallbackType string `pulumi:"effectiveVoiceCallbackType"`
	// EffectiveVoiceCallbackValue is the voice routing target Vonage currently applies.
	EffectiveVoiceCallbackValue string `pulumi:"effectiveVoiceCallbackValue"`
}

func (s *NumberState) Annotate(a infer.Annotator) {
	a.Describe(&s.Type, "Number type reported by Vonage, for example landline, mobile-lvn or landline-toll-free.")
	a.Describe(&s.Features, "Capabilities of the number, for example SMS, VOICE, MMS.")
	a.Describe(&s.MessagesCallbackType, "Effective inbound messages routing type (app or empty).")
	a.Describe(&s.MessagesCallbackValue, "Effective inbound messages routing target (usually the linked application ID).")
	a.Describe(&s.EffectiveVoiceCallbackType, "Voice routing type Vonage currently applies (sip, tel or app).")
	a.Describe(&s.EffectiveVoiceCallbackValue, "Voice routing target Vonage currently applies.")
}

func (r *Number) Annotate(a infer.Annotator) {
	a.Describe(&r, "A Vonage virtual number. Buying is billed; cancelling is permanent. Both are explicit inputs "+
		"(acquisition, cancelOnDelete).")
}

var _ infer.CustomResource[NumberArgs, NumberState] = (*Number)(nil)
var _ infer.CustomCheck[NumberArgs] = (*Number)(nil)
var _ infer.CustomRead[NumberArgs, NumberState] = (*Number)(nil)
var _ infer.CustomUpdate[NumberArgs, NumberState] = (*Number)(nil)
var _ infer.CustomDelete[NumberState] = (*Number)(nil)

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (r *Number) Check(ctx context.Context, req infer.CheckRequest) (infer.CheckResponse[NumberArgs], error) {
	args, failures, err := infer.DefaultCheck[NumberArgs](ctx, req.NewInputs)
	if err != nil || len(failures) > 0 {
		return infer.CheckResponse[NumberArgs]{Inputs: args, Failures: failures}, err
	}
	if len(args.Country) != 2 || strings.ToUpper(args.Country) != args.Country {
		failures = append(failures, p.CheckFailure{Property: "country", Reason: "must be a two-letter upper-case ISO country code, for example US"})
	}
	if !isDigits(args.Msisdn) {
		failures = append(failures, p.CheckFailure{Property: "msisdn", Reason: "must contain digits only, in international format without a leading plus"})
	}
	if args.Acquisition != "buy" && args.Acquisition != "adopt" {
		failures = append(failures, p.CheckFailure{Property: "acquisition", Reason: "must be \"buy\" or \"adopt\""})
	}
	if args.VoiceCallbackType != nil {
		switch *args.VoiceCallbackType {
		case "sip", "tel", "app":
		default:
			failures = append(failures, p.CheckFailure{Property: "voiceCallbackType", Reason: "must be sip, tel or app"})
		}
	}
	if (args.VoiceCallbackType == nil) != (args.VoiceCallbackValue == nil) {
		failures = append(failures, p.CheckFailure{Property: "voiceCallbackValue", Reason: "voiceCallbackType and voiceCallbackValue must be set together"})
	}
	if args.AppID != nil && *args.AppID != "" &&
		(args.MoHTTPURL != nil || args.VoiceCallbackType != nil || args.VoiceStatusCallback != nil) {
		failures = append(failures, p.CheckFailure{Property: "appId",
			Reason: "appId cannot be combined with moHttpUrl, voiceCallbackType/Value or voiceStatusCallback; the application owns routing"})
	}
	return infer.CheckResponse[NumberArgs]{Inputs: args, Failures: failures}, nil
}

func numberUpdateFrom(a NumberArgs) numberUpdate {
	return numberUpdate{
		AppID:               a.AppID,
		MoHTTPURL:           a.MoHTTPURL,
		VoiceCallbackType:   a.VoiceCallbackType,
		VoiceCallbackValue:  a.VoiceCallbackValue,
		VoiceStatusCallback: a.VoiceStatusCallback,
	}
}

func hasRouting(u numberUpdate) bool {
	return u.AppID != nil || u.MoHTTPURL != nil || u.VoiceCallbackType != nil || u.VoiceCallbackValue != nil || u.VoiceStatusCallback != nil
}

func numberStateFrom(args NumberArgs, n *wireOwnedNumber) NumberState {
	feats := n.Features
	if feats == nil {
		feats = []string{}
	}
	return NumberState{
		NumberArgs: args, Type: n.Type, Features: feats,
		MessagesCallbackType: n.MessagesCallbackType, MessagesCallbackValue: n.MessagesCallbackValue,
		EffectiveVoiceCallbackType: n.VoiceCallbackType, EffectiveVoiceCallbackValue: n.VoiceCallbackValue,
	}
}

func (r *Number) Create(ctx context.Context, req infer.CreateRequest[NumberArgs]) (infer.CreateResponse[NumberState], error) {
	a := req.Inputs
	id := a.Country + ":" + a.Msisdn
	if req.DryRun {
		return infer.CreateResponse[NumberState]{ID: id, Output: NumberState{NumberArgs: a, Features: []string{}}}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.CreateResponse[NumberState]{}, err
	}
	owned, err := c.FindOwnedNumber(ctx, a.Country, a.Msisdn)
	if err != nil {
		return infer.CreateResponse[NumberState]{}, fmt.Errorf("look up number %s: %w", a.Msisdn, err)
	}
	switch a.Acquisition {
	case "buy":
		if owned != nil {
			return infer.CreateResponse[NumberState]{}, fmt.Errorf(
				"number %s (%s) is already owned by this account; use acquisition=\"adopt\" to manage it without buying", a.Msisdn, a.Country)
		}
		if err := c.BuyNumber(ctx, a.Country, a.Msisdn); err != nil {
			return infer.CreateResponse[NumberState]{}, fmt.Errorf("buy number %s (%s): %w", a.Msisdn, a.Country, err)
		}
	case "adopt":
		if owned == nil {
			return infer.CreateResponse[NumberState]{}, fmt.Errorf(
				"number %s (%s) is not owned by this account; use acquisition=\"buy\" to purchase it", a.Msisdn, a.Country)
		}
	default:
		return infer.CreateResponse[NumberState]{}, fmt.Errorf("acquisition must be \"buy\" or \"adopt\", got %q", a.Acquisition)
	}
	if u := numberUpdateFrom(a); hasRouting(u) {
		if err := c.UpdateNumber(ctx, a.Country, a.Msisdn, u); err != nil {
			// The purchase (if any) already succeeded: report the partial state so the
			// number is tracked and can be fixed or cancelled by Pulumi.
			return infer.CreateResponse[NumberState]{ID: id, Output: NumberState{NumberArgs: a, Features: []string{}}},
				infer.ResourceInitFailedError{Reasons: []string{fmt.Sprintf("number %s acquired but configuring it failed: %v", a.Msisdn, err)}}
		}
	}
	n, err := c.FindOwnedNumber(ctx, a.Country, a.Msisdn)
	if err != nil || n == nil {
		// Eventual consistency right after a purchase: fall back to the known inputs.
		return infer.CreateResponse[NumberState]{ID: id, Output: NumberState{NumberArgs: a, Features: []string{}}}, nil
	}
	return infer.CreateResponse[NumberState]{ID: id, Output: numberStateFrom(a, n)}, nil
}

func (r *Number) Read(ctx context.Context, req infer.ReadRequest[NumberArgs, NumberState]) (infer.ReadResponse[NumberArgs, NumberState], error) {
	country, msisdn := splitNumberID(req.ID)
	c, err := clientFor(ctx)
	if err != nil {
		return infer.ReadResponse[NumberArgs, NumberState]{}, err
	}
	n, err := c.FindOwnedNumber(ctx, country, msisdn)
	if err != nil {
		return infer.ReadResponse[NumberArgs, NumberState]{}, fmt.Errorf("read number %s: %w", req.ID, err)
	}
	if n == nil {
		return infer.ReadResponse[NumberArgs, NumberState]{}, nil
	}
	args := req.Inputs
	if args.Country == "" { // import
		args.Country, args.Msisdn = country, msisdn
		args.Acquisition = "adopt"
	}
	// Refresh only the routing fields the user manages, so unmanaged defaults never diff.
	if n.AppID != "" || args.AppID != nil {
		if n.AppID != "" {
			args.AppID = strPtr(n.AppID)
		} else {
			args.AppID = nil
		}
	}
	if args.MoHTTPURL != nil || n.MoHTTPURL != "" {
		if n.MoHTTPURL != "" {
			args.MoHTTPURL = strPtr(n.MoHTTPURL)
		} else {
			args.MoHTTPURL = nil
		}
	}
	// Voice routing: managed (or imported) and not superseded by an application link.
	// An import supplies only the ID, so the program's inputs carry no country; in that
	// case adopt whatever routing Vonage reports instead of preserving the program's absence.
	imported := isImportRead(req.Inputs)
	if n.AppID == "" && (imported || args.VoiceCallbackType != nil || args.VoiceCallbackValue != nil) {
		args.VoiceCallbackType = nonEmptyPtr(n.VoiceCallbackType)
		args.VoiceCallbackValue = nonEmptyPtr(n.VoiceCallbackValue)
	}
	// The status callback is only observable when the list endpoint reports it.
	if n.VoiceStatusCallback != nil && (imported || args.VoiceStatusCallback != nil || *n.VoiceStatusCallback != "") {
		args.VoiceStatusCallback = nonEmptyPtr(*n.VoiceStatusCallback)
	}
	return infer.ReadResponse[NumberArgs, NumberState]{ID: req.ID, Inputs: args, State: numberStateFrom(args, n)}, nil
}

func nonEmptyPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func splitNumberID(id string) (country, msisdn string) {
	if i := strings.Index(id, ":"); i >= 0 {
		return id[:i], id[i+1:]
	}
	return "", id
}

func (r *Number) Update(ctx context.Context, req infer.UpdateRequest[NumberArgs, NumberState]) (infer.UpdateResponse[NumberState], error) {
	a, old := req.Inputs, req.State.NumberArgs
	if req.DryRun {
		st := req.State
		st.NumberArgs = a
		return infer.UpdateResponse[NumberState]{Output: st}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.UpdateResponse[NumberState]{}, err
	}
	u := numberUpdateFrom(a)
	// Removing a managed value clears it with an empty form value.
	clear := func(now, before *string) *string {
		if now == nil && before != nil && *before != "" {
			return strPtr("")
		}
		return now
	}
	u.AppID = clear(a.AppID, old.AppID)
	u.MoHTTPURL = clear(a.MoHTTPURL, old.MoHTTPURL)
	u.VoiceStatusCallback = clear(a.VoiceStatusCallback, old.VoiceStatusCallback)
	// Removing voice routing must stop the number routing voice calls. Skip when
	// the number is being linked to an application, which supersedes it.
	if a.AppID == nil {
		u.VoiceCallbackType = clear(a.VoiceCallbackType, old.VoiceCallbackType)
		u.VoiceCallbackValue = clear(a.VoiceCallbackValue, old.VoiceCallbackValue)
	}
	if hasRouting(u) || u.VoiceCallbackValue != nil {
		if err := c.UpdateNumber(ctx, a.Country, a.Msisdn, u); err != nil {
			return infer.UpdateResponse[NumberState]{}, fmt.Errorf("update number %s: %w", a.Msisdn, err)
		}
	}
	n, err := c.FindOwnedNumber(ctx, a.Country, a.Msisdn)
	if err != nil {
		return infer.UpdateResponse[NumberState]{}, fmt.Errorf("refresh number %s: %w", a.Msisdn, err)
	}
	if n == nil {
		return infer.UpdateResponse[NumberState]{}, fmt.Errorf("number %s (%s) is no longer owned by this account", a.Msisdn, a.Country)
	}
	return infer.UpdateResponse[NumberState]{Output: numberStateFrom(a, n)}, nil
}

func (r *Number) Delete(ctx context.Context, req infer.DeleteRequest[NumberState]) (infer.DeleteResponse, error) {
	if !req.State.CancelOnDelete {
		// Explicitly not cancelling: stop managing the number and leave it (and its billing) in place.
		return infer.DeleteResponse{}, nil
	}
	c, err := clientFor(ctx)
	if err != nil {
		return infer.DeleteResponse{}, err
	}
	country, msisdn := req.State.Country, req.State.Msisdn
	n, err := c.FindOwnedNumber(ctx, country, msisdn)
	if err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("look up number %s before cancelling: %w", msisdn, err)
	}
	if n == nil {
		return infer.DeleteResponse{}, nil // already gone
	}
	if err := c.CancelNumber(ctx, country, msisdn); err != nil {
		return infer.DeleteResponse{}, fmt.Errorf("cancel number %s (%s): %w", msisdn, country, err)
	}
	return infer.DeleteResponse{}, nil
}

// isImportRead reports whether a Read is an import (the engine passes empty
// inputs) rather than a refresh of a managed Number, whose country is required.
func isImportRead(in NumberArgs) bool { return in.Country == "" }
