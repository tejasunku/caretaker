package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tejasunku/caretaker/internal/image"
	"github.com/tejasunku/caretaker/internal/lineage"
	"github.com/tejasunku/caretaker/internal/registry"
)

func (a *App) lineageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lineage",
		Short: "Search the registry's append-only lineage log",
	}
	cmd.AddCommand(a.lineageLogCmd(), a.lineageChildrenCmd(), a.lineageShowCmd())
	return cmd
}

// lineageLogCmd prints every lineage entry recorded for one repository.
func (a *App) lineageLogCmd() *cobra.Command {
	var service, repo string
	cmd := &cobra.Command{
		Use:   "log",
		Short: "Print the whole lineage log for a repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			repository, err := a.lineageRepository(service, repo)
			if err != nil {
				return err
			}
			entries, err := a.lineageReader().Log(ctx, repository)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Fprintf(a.Out, "no lineage entries for %s\n", repository)
				return nil
			}
			rows := make([][]string, 0, len(entries))
			for _, e := range entries {
				rows = append(rows, []string{
					shortRef(e.ParentDigest), shortRef(e.ChildDigest),
					shortRef(e.BaseDigest), e.BaseVersion, e.Service, e.PatchedAt,
				})
			}
			a.writeTable([]string{"PARENT", "CHILD", "BASE", "VERSION", "SERVICE", "PATCHED AT"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&service, "service", "", "service whose repository is searched")
	cmd.Flags().StringVar(&repo, "repo", "", "repository (default: from --service)")
	return cmd
}

// lineageChildrenCmd answers "what was patched from this image?", directly and
// indirectly, by walking the log's parent links and the base index.
func (a *App) lineageChildrenCmd() *cobra.Command {
	var service, repo string
	cmd := &cobra.Command{
		Use:   "children <image>",
		Short: "List child images of an image, directly and indirectly",
		Long: "Resolves the image's lineage log and walks it: direct children " +
			"(the recorded parent link), then their children, and so on. Children " +
			"that can only be found through the base digest — because their parent " +
			"image was deleted — are included too.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			reg := a.registryClient()
			ref, err := a.parseLineageRef(ctx, reg, args[0], service, repo)
			if err != nil {
				return err
			}
			entries, err := a.lineageReader().Log(ctx, ref.Repository)
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Fprintf(a.Out, "no lineage entries for %s\n", ref.Repository)
				return nil
			}
			depths := childDepths(entries, ref.Digest)
			if len(depths) == 0 {
				fmt.Fprintf(a.Out, "no children recorded for %s\n", shortRef(ref.String()))
				return nil
			}
			rows := make([][]string, 0, len(depths))
			for _, e := range sortedByDepth(entries, depths) {
				rows = append(rows, []string{
					fmt.Sprintf("%d", depths[e.ChildDigest]),
					shortRef(e.ChildDigest), shortRef(e.ParentDigest),
					shortRef(e.BaseDigest), e.BaseVersion, e.Service, e.PatchedAt,
				})
			}
			a.writeTable([]string{"DEPTH", "CHILD", "PARENT", "BASE", "VERSION", "SERVICE", "PATCHED AT"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&service, "service", "", "service whose repository is searched")
	cmd.Flags().StringVar(&repo, "repo", "", "repository (default: from --service)")
	return cmd
}

// lineageShowCmd prints the metadata recorded on one patched child.
func (a *App) lineageShowCmd() *cobra.Command {
	var service, repo string
	cmd := &cobra.Command{
		Use:   "show <child-image>",
		Short: "Show the lineage metadata recorded on a patched image",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			reg := a.registryClient()
			ref, err := a.parseLineageRef(ctx, reg, args[0], service, repo)
			if err != nil {
				return err
			}
			meta, ok, err := a.lineageReader().Metadata(ctx, ref.String())
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("%s carries no caretaker lineage metadata", shortRef(ref.String()))
			}
			rows := [][]string{
				{"service", meta.Service},
				{"base", meta.BaseDigest},
				{"base version", meta.BaseVersion},
				{"parent", meta.ParentDigest},
				{"source", meta.SourceDigest},
				{"patched at", meta.PatchedAt},
			}
			a.writeTable([]string{"FIELD", "VALUE"}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&service, "service", "", "service whose repository holds the image")
	cmd.Flags().StringVar(&repo, "repo", "", "repository (default: from --service)")
	return cmd
}

func (a *App) lineageReader() *lineage.RecorderClient {
	return &lineage.RecorderClient{Registry: a.registryClient()}
}

// lineageRepository maps --service/--repo onto a repository path.
func (a *App) lineageRepository(service, repo string) (string, error) {
	if r := strings.TrimSpace(repo); r != "" {
		return r, nil
	}
	if s := strings.TrimSpace(service); s != "" {
		return a.cfg.Repository(s), nil
	}
	return "", errString("pass --service <name> or --repo <repository>")
}

// parseLineageRef accepts a full repo@digest reference, or a bare digest plus
// --service/--repo. Tags resolve only with --allow-tags.
func (a *App) parseLineageRef(ctx context.Context, reg *registry.Client, raw, service, repo string) (image.Ref, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "sha256:") || strings.HasPrefix(raw, "sha512:") {
		repository, err := a.lineageRepository(service, repo)
		if err != nil {
			return image.Ref{}, err
		}
		raw = repository + "@" + raw
	}
	ref, err := image.Parse(raw)
	if err != nil {
		return image.Ref{}, err
	}
	if ref.Digest == "" {
		if !a.allowTags() {
			return image.Ref{}, fmt.Errorf("reference %q has no digest; caretaker is digest-first (use --allow-tags for tag input)", raw)
		}
		digest, err := reg.Resolve(ctx, raw)
		if err != nil {
			return image.Ref{}, err
		}
		ref.Digest = digest
		ref.Tag = ""
	}
	return ref, nil
}

// childDepths maps every descendant of root to its depth (direct children are
// 1). It walks the log's parent links, then folds in anything reachable only
// through the base digest, which is how children survive the deletion of their
// parent image. Nil when root has no recorded children.
func childDepths(entries []lineage.Entry, root string) map[string]int {
	if root == "" {
		return nil
	}
	byParent := map[string][]lineage.Entry{}
	baseEntries := map[string][]lineage.Entry{}
	for _, e := range entries {
		if e.ChildDigest == "" {
			continue
		}
		byParent[e.ParentDigest] = append(byParent[e.ParentDigest], e)
		byParent[e.SourceDigest] = append(byParent[e.SourceDigest], e)
		if e.BaseDigest != "" {
			baseEntries[e.BaseDigest] = append(baseEntries[e.BaseDigest], e)
		}
	}

	depths := map[string]int{}
	queue := []string{root}
	expand := func() {
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			for _, e := range byParent[current] {
				if _, seen := depths[e.ChildDigest]; seen {
					continue
				}
				depths[e.ChildDigest] = depths[current] + 1
				queue = append(queue, e.ChildDigest)
			}
		}
	}
	expand()

	// When the target is an unpatched base (never itself a recorded child),
	// also fold in every entry carrying that base digest: their parent link may
	// point at a legacy patched image that predates the log. When the target is
	// itself a patched child we stay on the parent links so siblings of a
	// mid-chain image are not reported as its children.
	isChild := false
	for _, e := range entries {
		if e.ChildDigest == root {
			isChild = true
			break
		}
	}
	if !isChild {
		for _, e := range baseEntries[root] {
			if _, seen := depths[e.ChildDigest]; seen {
				continue
			}
			depths[e.ChildDigest] = 1
			queue = append(queue, e.ChildDigest)
		}
		expand()
	}

	if len(depths) == 0 {
		return nil
	}
	return depths
}

// sortedByDepth orders matched entries by depth, then patch time.
func sortedByDepth(entries []lineage.Entry, depths map[string]int) []lineage.Entry {
	matched := make([]lineage.Entry, 0, len(depths))
	for _, e := range entries {
		if _, ok := depths[e.ChildDigest]; ok {
			matched = append(matched, e)
		}
	}
	sort.SliceStable(matched, func(i, j int) bool {
		di, dj := depths[matched[i].ChildDigest], depths[matched[j].ChildDigest]
		if di != dj {
			return di < dj
		}
		return matched[i].PatchedAt < matched[j].PatchedAt
	})
	return matched
}
