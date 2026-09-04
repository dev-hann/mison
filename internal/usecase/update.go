package usecase

import (
	"fmt"
	"strings"

	"github.com/dev-hann/mison/internal/repo/miserepo"
)

// MisonRepo is the GitHub repo releases are published to.
const MisonRepo = "dev-hann/mison"

// RunUpdate re-resolves fuzzy version selectors of declared tools
// (node "latest", "22" — exact pins stay untouched), installs the new
// versions, and pushes the refreshed lockfile. This is the explicit
// update path DESIGN #12 always implied: sync never bumps.
func (f *Flows) RunUpdate(args []string, dryRun bool, policy ConflictPolicy) error {
	guard, err := f.acquireRunLock()
	if err != nil {
		f.UI.Fail(err.Error())
		return err
	}
	defer guard.Release()

	if _, err := f.layout().Ensure(); err != nil {
		return err
	}
	if err := f.ensureMise(); err != nil {
		return err
	}

	candidates, err := f.Mise.BumpDryRun()
	if err != nil {
		return err
	}
	if len(args) > 0 {
		want := map[string]bool{}
		for _, a := range args {
			want[a] = true
		}
		var filtered []miserepo.BumpCandidate
		for _, c := range candidates {
			if want[c.Name] {
				filtered = append(filtered, c)
				delete(want, c.Name)
			}
		}
		for missing := range want {
			f.UI.Warn(missing + ": no update available (pinned or already current)")
		}
		candidates = filtered
	}
	if len(candidates) == 0 {
		f.UI.Step("All declared tools are up to date")
		return nil
	}

	var names []string
	var msgParts []string
	for _, c := range candidates {
		names = append(names, c.Name)
		msgParts = append(msgParts, fmt.Sprintf("%s %s → %s",
			c.Name, strings.Join(c.OldVersions, ","), strings.Join(c.NewVersions, ",")))
		f.UI.Line("  " + msgParts[len(msgParts)-1])
	}

	if dryRun {
		return nil
	}
	if !f.Ask.Confirm(fmt.Sprintf("Update %d tool(s)?", len(candidates))) {
		f.UI.Line("aborted — nothing changed")
		return nil
	}

	bumpArgs := append([]string{"lock", "--global", "--bump"}, names...)
	if err := f.Mise.Exec(bumpArgs...); err != nil {
		return err
	}
	f.UI.Step("Installing new versions")
	if err := f.Mise.Exec("install"); err != nil {
		return fmt.Errorf("%w — lockfile not pushed; retry with mison update", err)
	}
	f.refreshLock()
	return f.commitAndPush("update: "+strings.Join(msgParts, ", "), policy)
}

// RunUpgrade updates the mison binary itself via the official
// installer (checksum-verified, mison.old backup kept). Plain HTTP —
// independent of gh auth.
func (f *Flows) RunUpgrade(currentVersion string) error {
	if currentVersion == "dev" {
		return fmt.Errorf("development build — release comparison is meaningless; reinstall from a release tag or rebuild")
	}
	latest, err := f.Gh.LatestReleaseTag(MisonRepo)
	if err != nil {
		return err
	}
	// goreleaser injects "0.5.0" (no v); the API returns "v0.5.0" —
	// normalize before comparing so equal versions aren't reinstalled
	trim := func(v string) string { return strings.TrimPrefix(v, "v") }

	if trim(latest) != trim(currentVersion) {
		f.UI.Step(fmt.Sprintf("Upgrading mison %s → %s", currentVersion, latest))
		if err := f.Gh.RunMisonInstaller(); err != nil {
			return err
		}
		f.UI.Line("Previous binary kept as ~/.local/bin/mison.old (rollback: cp it back)")
		f.UI.Step("Upgraded to " + latest)
	} else {
		f.UI.Step("mison is up to date (" + currentVersion + ")")
	}

	// mise is the other binary mison depends on — upgrade owns it too
	// (decision #12's explicit path; sync never touches mise).
	before, _ := f.Mise.Version()
	if err := f.Mise.Exec("self-update"); err != nil {
		f.UI.Warn("mise self-update unavailable (" + err.Error() + ") — if mise came from brew: brew upgrade mise")
		return nil
	}
	after, _ := f.Mise.Version()
	if before != "" && after != "" && before != after {
		f.UI.Step("mise " + before + " → " + after)
	} else {
		f.UI.Step("mise is up to date" + func() string {
			if after != "" {
				return " (" + after + ")"
			}
			return ""
		}())
	}
	return nil
}
