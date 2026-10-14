package cli

import (
	"context"
	"fmt"
	"kc/declarativeindex/client"
	"kc/kernel"
	"sort"
	"strings"
)

type executor struct{ client client.Client }

func New(remote client.Client) Executor { return &executor{client: remote} }
func flagString(flags map[string]any, name string) string {
	if flags[name] == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(flags[name]))
}
func (e *executor) ValidateFlags(path string, flags map[string]any) error {
	entry, ok := command(path)
	if !ok {
		return nil
	}
	allowed := map[string]bool{}
	for _, name := range append(append([]string(nil), entry.Flags...), strings.Fields("server home as request-id trace-id span-id parent-span-id on-behalf-of help")...) {
		allowed[name] = true
	}
	var unknown []string
	for name := range flags {
		if !allowed[name] {
			unknown = append(unknown, name)
		}
	}
	sort.Strings(unknown)
	if len(unknown) > 0 {
		return kernel.Fail(kernel.ErrUsageInvalid, "unknown flag --%s for kc %s", unknown[0], path)
	}
	for name, value := range flags {
		if _, multiple := value.([]string); multiple {
			return kernel.Fail(kernel.ErrUsageInvalid, "--%s must be supplied once", name)
		}
		if _, missing := value.(bool); missing && name != "help" {
			return kernel.Fail(kernel.ErrUsageInvalid, "--%s requires a value", name)
		}
	}
	return nil
}
func (e *executor) Validate(path string, flags map[string]any) error {
	if err := e.ValidateFlags(path, flags); err != nil {
		return err
	}
	if _, ok := command(path); !ok || flags["help"] == true {
		return nil
	}
	if flagString(flags, "repo") == "" {
		return kernel.Fail(kernel.ErrUsageInvalid, "missing --repo")
	}
	return nil
}
func (e *executor) Execute(ctx context.Context, path string, flags map[string]any, options client.RequestOptions) (any, error) {
	if _, ok := command(path); !ok {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "declarative-index CLI does not implement %s", path)
	}
	if err := e.Validate(path, flags); err != nil {
		return nil, err
	}
	if flags["help"] == true {
		help := leafHelpEntries()[path]
		return help.Description + "\n\n" + help.Usage, nil
	}
	if e.client == nil {
		return nil, kernel.Fail(kernel.ErrCapabilityUnsatisfied, "declarative-index CLI requires an authenticated HTTP client")
	}
	var output any
	var err error
	request := client.ProjectionRequest{Repository: flagString(flags, "repo"), Commit: flagString(flags, "commit"), Ref: flagString(flags, "ref")}
	switch path {
	case "operations projection describe":
		err = e.client.DescribeProjection(ctx, request, options, &output)
	case "operations projection sync":
		err = e.client.SyncProjection(ctx, request, options, &output)
	case "operations projection notice":
		notice := client.ProjectionNotifyRequest{Repository: request.Repository, Ref: request.Ref, SourceRevision: flagString(flags, "source-revision")}
		if object := flagString(flags, "object"); object != "" {
			kind := flagString(flags, "kind")
			if kind == "" && flagString(flags, "aspect") != "" {
				kind = "Aspect"
			}
			notice.Address = &client.ProjectionNoticeAddress{Kind: kind, ObjectID: object, AspectName: flagString(flags, "aspect")}
		}
		err = e.client.NotifyProjection(ctx, notice, options, &output)
	}
	return output, err
}
