package invocation

import (
	"kc/kernel"
	objectmodel "kc/object"
	objectservice "kc/object/service"
	"kc/snapshot"
)

const referenceAspectFlag = "_object-reference-aspect"

// A locator selects JSON content, not an authorization namespace. Only fixed
// maintenance coordinates can prove which existing Aspect grant applies.
func prepareReferenceAuthorization(cx *Invocation, action string) error {
	delete(cx.Flags, referenceAspectFlag)
	ref, err := referenceFromFlags(cx.Flags)
	if err != nil || ref == nil || ref.Locator == "" || ServingWorkspace(cx.Flags) || OwnerBypass(cx.Flags) {
		return err
	}
	if action != "knowledge.read" && action != "writer.commit" {
		return nil
	}
	tokens, err := objectmodel.PointerTokens(ref.Locator)
	if err != nil {
		return err
	}
	policy, err := ReadAllow(cx.Home)
	if err != nil {
		return nil // The ordinary gate reports unavailable policy without reading content.
	}
	query := AllowQuery{Principal: cx.Flag("as"), Action: action, Repo: string(ref.Repository), Ref: cx.Flag("ref"), Object: string(ref.Object)}
	if _, allowed := MatchAllow(policy.Rules, query); allowed {
		return nil
	}
	query.Aspect = tokens[0]
	if _, candidate := MatchAllow(policy.Rules, query); !candidate {
		return nil
	}
	commit, err := referenceAuthorizationBasis(cx, action, *ref)
	if err != nil {
		return err
	}
	root := *ref
	root.Locator = ""
	coordinates, err := cx.WS.Reader.PlanReference(root, commit)
	if err != nil {
		return err
	}
	if action != "knowledge.read" {
		address, err := objectservice.ResolveMaintenanceAddress(*ref, objectmodel.AddressKind(cx.Flag("kind")), coordinates)
		if err != nil {
			return err
		}
		cx.Flags[referenceAspectFlag] = address.AspectName
		return nil
	}
	for _, address := range coordinates {
		if objectmodel.IsEntityBlob(address) {
			return nil // A same-named field inside a whole value is not an Aspect.
		}
	}
	// Composite and virtual Bound Aspects share their declared Aspect namespace.
	// Actual content and Bound Schema proof remain scoped in the reader/runtime.
	cx.Flags[referenceAspectFlag] = tokens[0]
	return nil
}

func referenceAuthorizationBasis(cx *Invocation, action string, ref objectmodel.Reference) (kernel.CommitID, error) {
	if action == "knowledge.read" {
		_, commit, err := PinCommit(cx.WS, cx.Flags)
		if err == nil {
			cx.Flags["commit"] = string(commit)
		}
		return commit, err
	}
	base := kernel.CommitID(cx.Flag("base"))
	if prior, ok := cx.WS.Writer.Lookup(cx.Flag("command-id")); ok && prior.Request.RepositoryID == ref.Repository && prior.Request.TargetRef == snapshot.RefOrDefault(cx.Flag("ref")) {
		if base == "" {
			base = prior.Request.BaseCommit
		}
	}
	if base == "" {
		repo, err := cx.WS.Reader.Require(ref.Repository, kernel.ErrKnowledgeRefUnresolved)
		if err != nil {
			return "", err
		}
		base, err = repo.Head(snapshot.RefOrDefault(cx.Flag("ref")))
		if err != nil {
			return "", err
		}
		cx.Flags["base"] = string(base)
	}
	return base, nil
}
